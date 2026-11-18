// Package task xu ly yeu cau/cong viec trong chat: tao task, nhan task,
// xu ly (tam dung/tiep tuc/treo/hoan thanh), chia task, doi phan cong va phieu sua.
//
// Nguyen tac:
//   - Moi nguoi duoc giao viec = 1 dong task_assignments ("card cua ai hien cua nguoi do").
//   - Trang thai task TONG duoc suy ra tu cac card (domain.DeriveTaskStatus), khong set tay.
//   - Moi thay doi deu: ghi task_events (lich su) + dang 1 tin nhan he thong vao thread + publish realtime.
package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
)

// MessagePoster la cua de task dang tin nhan he thong (do message.Service cai dat).
// Dung interface de test task service khong can tang message.
type MessagePoster interface {
	PostSystem(ctx context.Context, in message.SystemMessageInput) (*domain.Message, error)
}

type Service struct {
	store  repository.Store
	convs  *conversation.Service
	poster MessagePoster
	hub    *realtime.Hub
	log    *slog.Logger
}

func New(store repository.Store, convs *conversation.Service, poster MessagePoster, hub *realtime.Hub) *Service {
	return &Service{
		store: store, convs: convs, poster: poster, hub: hub,
		log: slog.Default().With("component", "service.task"),
	}
}

// ---------------- tao task ----------------

type CreateInput struct {
	ActorID        uuid.UUID
	ConversationID uuid.UUID
	// ThreadID: neu nil, card task vua dang se tro thanh thread goc cua yeu cau.
	ThreadID     *uuid.UUID
	Title        string
	Description  string
	TeamID       *uuid.UUID
	TeamCode     string // tien cho bot: "design" | "edit" ...
	Quantity     int
	Quota        string
	Deadline     *time.Time
	Priority     domain.TaskPriority
	AssigneeIDs  []uuid.UUID // rong => tu goi y 1 nguoi trong to
	ParentTaskID *uuid.UUID
	SubmissionID *uuid.UUID
	Metadata     domain.JSONMap
}

// Create tao yeu cau trong hoi thoai, tu goi y nguoi phu trach neu chua chon.
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Task, error) {
	if _, err := s.convs.EnsureMember(ctx, in.ConversationID, in.ActorID); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, apperr.Invalid("task_title_required", "Yeu cau phai co tieu de")
	}
	if in.Quantity <= 0 {
		in.Quantity = 1
	}
	priority := in.Priority
	if priority == "" {
		priority = domain.PriorityNormal
	}
	if !priority.Valid() {
		return nil, apperr.Invalid("invalid_priority", "priority phai la low|normal|high|urgent")
	}

	// 1) To (tuy chon): nhan theo id hoac code.
	team, err := s.resolveTeam(ctx, in.TeamID, in.TeamCode)
	if err != nil {
		return nil, err
	}

	// 2) Nguoi phu trach: lay danh sach duoc chi dinh, neu khong co thi goi y tu to.
	assigneeIDs := uniqueIDs(in.AssigneeIDs)
	suggested := false
	if len(assigneeIDs) == 0 && team != nil {
		if sug, err := s.SuggestAssignee(ctx, team.ID); err == nil && sug != nil {
			assigneeIDs = []uuid.UUID{sug.UserID}
			suggested = true
		}
	}
	// Nguoi nhan viec phai o trong hoi thoai moi thay thread => tu them vao nhom.
	if err := s.ensureInConversation(ctx, in.ConversationID, assigneeIDs); err != nil {
		return nil, err
	}

	var teamID *uuid.UUID
	if team != nil {
		teamID = &team.ID
	}

	meta := in.Metadata
	if meta == nil {
		meta = domain.JSONMap{}
	}
	if suggested {
		meta["assignee_suggested"] = true
	}

	t := &domain.Task{
		ID:             uuid.New(),
		ConversationID: in.ConversationID,
		ThreadID:       in.ThreadID,
		ParentTaskID:   in.ParentTaskID,
		SubmissionID:   in.SubmissionID,
		TeamID:         teamID,
		Title:          title,
		Description:    in.Description,
		Quantity:       in.Quantity,
		Quota:          in.Quota,
		Deadline:       in.Deadline,
		Priority:       priority,
		Status:         domain.TaskWaitingAccept,
		CreatedBy:      in.ActorID,
		Metadata:       meta,
		CreatedAt:      time.Now().UTC(),
	}
	for _, uid := range assigneeIDs {
		t.Assignments = append(t.Assignments, domain.TaskAssignment{
			ID:         uuid.New(),
			UserID:     uid,
			Role:       domain.AssignPrimary,
			Status:     domain.AssignPending,
			AssignedBy: &in.ActorID,
			AssignedAt: time.Now().UTC(),
		})
	}

	// 3) Ghi DB trong 1 transaction: ma task + task + card + event.
	err = s.store.Tx().Do(ctx, func(ctx context.Context) error {
		code, err := s.store.Tasks().NextTaskCode(ctx)
		if err != nil {
			return err
		}
		t.Code = code
		if err := s.store.Tasks().Create(ctx, t); err != nil {
			return err
		}
		return s.store.Tasks().CreateEvent(ctx, &domain.TaskEvent{
			TaskID:   t.ID,
			Type:     domain.EventTaskCreated,
			ActorID:  &in.ActorID,
			ToStatus: string(t.Status),
			Message:  fmt.Sprintf("Tao yeu cau %s", t.Code),
			Payload: domain.JSONMap{
				"assignee_ids": idStrings(assigneeIDs),
				"suggested":    suggested,
			},
		})
	})
	if err != nil {
		return nil, err
	}

	full, err := s.store.Tasks().GetByID(ctx, t.ID)
	if err != nil {
		return nil, err
	}

	// 4) Dang card vao hoi thoai; neu chua co thread thi card nay LA thread goc.
	card, err := s.poster.PostSystem(ctx, message.SystemMessageInput{
		ConversationID: full.ConversationID,
		ThreadID:       full.ThreadID,
		Content:        RenderCard(full),
		ActorID:        &in.ActorID,
		Metadata:       cardMetadata(full),
	})
	if err != nil {
		s.log.Error("dang card task that bai", "task", full.Code, "err", err)
	} else if full.ThreadID == nil {
		if err := s.store.Tasks().UpdateTask(ctx, full.ID, map[string]any{"thread_id": card.ID}); err != nil {
			s.log.Error("gan thread cho task that bai", "task", full.Code, "err", err)
		} else {
			full.ThreadID = &card.ID
		}
	}

	s.publish(ctx, realtime.EventTaskCreated, full, domain.JSONMap{"actor_id": in.ActorID.String()})
	return full, nil
}

// ---------------- doc ----------------

func (s *Service) Get(ctx context.Context, actorID, taskID uuid.UUID) (*domain.Task, error) {
	t, err := s.store.Tasks().GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, t.ConversationID, actorID); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Service) GetByCode(ctx context.Context, actorID uuid.UUID, code string) (*domain.Task, error) {
	t, err := s.store.Tasks().GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, t.ConversationID, actorID); err != nil {
		return nil, err
	}
	return t, nil
}

type ListInput struct {
	ActorID        uuid.UUID
	ConversationID *uuid.UUID
	ThreadID       *uuid.UUID
	TeamID         *uuid.UUID
	AssigneeID     *uuid.UUID
	CreatedBy      *uuid.UUID
	Statuses       []domain.TaskStatus
	// WaitingAccept = true: chi lay task da giao nhung chua nhan ("Cho nhan").
	WaitingAccept bool
	// MineOnly = true: chi lay task minh duoc giao.
	MineOnly bool
	Query    string
}

func (s *Service) List(ctx context.Context, in ListInput, p paging.Page) ([]domain.Task, paging.PageInfo, error) {
	f := repository.TaskFilter{
		MemberUserID:   in.ActorID,
		ConversationID: in.ConversationID,
		ThreadID:       in.ThreadID,
		TeamID:         in.TeamID,
		CreatedBy:      in.CreatedBy,
		AssigneeID:     in.AssigneeID,
		Statuses:       in.Statuses,
		Query:          in.Query,
	}
	if in.MineOnly {
		f.AssigneeID = &in.ActorID
	}
	if in.WaitingAccept {
		f.AssignmentStatuses = []domain.AssignmentStatus{domain.AssignPending}
	}

	rows, err := s.store.Tasks().List(ctx, f, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, func(t domain.Task) (time.Time, string) {
		return t.CreatedAt, t.ID.String()
	})
	return items, info, nil
}

func (s *Service) Events(ctx context.Context, actorID, taskID uuid.UUID, p paging.Page) ([]domain.TaskEvent, paging.PageInfo, error) {
	if _, err := s.Get(ctx, actorID, taskID); err != nil {
		return nil, paging.PageInfo{}, err
	}
	rows, err := s.store.Tasks().ListEvents(ctx, taskID, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, func(e domain.TaskEvent) (time.Time, string) {
		return e.CreatedAt, e.ID.String()
	})
	return items, info, nil
}

// ---------------- helper dung chung ----------------

// requireManager: chi owner/admin cua nhom hoac nguoi tao task moi duoc dieu chinh.
func (s *Service) requireManager(ctx context.Context, t *domain.Task, actorID uuid.UUID) error {
	member, err := s.convs.EnsureMember(ctx, t.ConversationID, actorID)
	if err != nil {
		return err
	}
	if t.CreatedBy == actorID ||
		member.Role == domain.MemberOwner || member.Role == domain.MemberAdmin {
		return nil
	}
	return apperr.Forbidden("require_task_manager",
		"Chi tao truong/tro ly hoac nguoi tao yeu cau moi duoc dieu chinh task")
}

// syncStatus suy lai trang thai task tu cac card va ghi DB neu thay doi.
func (s *Service) syncStatus(ctx context.Context, t *domain.Task) (domain.TaskStatus, error) {
	assignments, err := s.store.Tasks().ListAssignments(ctx, t.ID)
	if err != nil {
		return t.Status, err
	}
	next := domain.DeriveTaskStatus(assignments)
	if t.Status == domain.TaskCancelled {
		return t.Status, nil
	}
	if next == t.Status {
		return next, nil
	}

	fields := map[string]any{"status": next}
	switch next {
	case domain.TaskCompleted:
		now := time.Now().UTC()
		fields["completed_at"] = now
	default:
		fields["completed_at"] = nil
	}
	if err := s.store.Tasks().UpdateTask(ctx, t.ID, fields); err != nil {
		return t.Status, err
	}
	t.Status = next
	return next, nil
}

// notify: ghi lich su + dang tin he thong vao thread + publish realtime.
type notifyInput struct {
	Task         *domain.Task
	ActorID      uuid.UUID
	EventType    domain.TaskEventType
	FromStatus   string
	ToStatus     string
	Message      string
	AssignmentID *uuid.UUID
	RevisionID   *uuid.UUID
	Payload      domain.JSONMap
}

func (s *Service) notify(ctx context.Context, in notifyInput) {
	t := in.Task
	if err := s.store.Tasks().CreateEvent(ctx, &domain.TaskEvent{
		TaskID:       t.ID,
		AssignmentID: in.AssignmentID,
		RevisionID:   in.RevisionID,
		Type:         in.EventType,
		ActorID:      &in.ActorID,
		FromStatus:   in.FromStatus,
		ToStatus:     in.ToStatus,
		Message:      in.Message,
		Payload:      in.Payload,
	}); err != nil {
		s.log.Error("ghi task event that bai", "task", t.Code, "err", err)
	}

	meta := domain.JSONMap{
		"kind":        "task_event",
		"event_type":  string(in.EventType),
		"task_id":     t.ID.String(),
		"task_code":   t.Code,
		"task_status": string(t.Status),
	}
	for k, v := range in.Payload {
		meta[k] = v
	}
	if _, err := s.poster.PostSystem(ctx, message.SystemMessageInput{
		ConversationID: t.ConversationID,
		ThreadID:       t.ThreadID,
		Content:        in.Message,
		ActorID:        &in.ActorID,
		Metadata:       meta,
	}); err != nil {
		s.log.Error("dang tin he thong that bai", "task", t.Code, "err", err)
	}

	s.publish(ctx, realtime.EventTaskUpdated, t, meta)
}

func (s *Service) publish(ctx context.Context, evType realtime.EventType, t *domain.Task, extra domain.JSONMap) {
	recipients, err := s.convs.MemberUserIDs(ctx, t.ConversationID)
	if err != nil {
		s.log.Error("lay thanh vien hoi thoai that bai", "err", err)
		return
	}
	full, err := s.store.Tasks().GetByID(ctx, t.ID)
	if err != nil {
		full = t
	}
	payload := map[string]any{"task": full}
	for k, v := range extra {
		payload[k] = v
	}
	s.hub.Publish(ctx, realtime.NewEvent(evType, t.ConversationID.String(), recipients, payload))
}

// ensureInConversation tu them nguoi nhan viec vao nhom (de ho thay thread/card).
func (s *Service) ensureInConversation(ctx context.Context, convID uuid.UUID, userIDs []uuid.UUID) error {
	var missing []domain.ConversationMember
	for _, uid := range userIDs {
		if _, err := s.convs.EnsureMember(ctx, convID, uid); err != nil {
			if isForbidden(err) || isNotFound(err) {
				missing = append(missing, domain.ConversationMember{
					ID: uuid.New(), ConversationID: convID, UserID: uid, Role: domain.MemberNormal,
				})
				continue
			}
			return err
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return s.store.Conversations().AddMembers(ctx, missing)
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func idStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func isNotFound(err error) bool { return kindOf(err) == apperr.KindNotFound }

func isForbidden(err error) bool { return kindOf(err) == apperr.KindForbidden }

func kindOf(err error) apperr.Kind {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return apperr.KindInternal
}
