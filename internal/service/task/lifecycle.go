package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

// ActionInput la input chung cho cac thao tac tren card task.
// AssignmentID = nil => tac dong len card cua chinh nguoi goi.
type ActionInput struct {
	ActorID      uuid.UUID
	TaskID       uuid.UUID
	AssignmentID *uuid.UUID
	Note         string
	// Chi dung cho Accept ("Design chon dinh muc va so luong truoc khi nhan").
	Quota    string
	Quantity int
}

// Accept — nhan task. Bat buoc chon dinh muc + so luong.
func (s *Service) Accept(ctx context.Context, in ActionInput) (*domain.Task, error) {
	t, a, err := s.loadForAction(ctx, in)
	if err != nil {
		return nil, err
	}
	if a.Status != domain.AssignPending {
		return nil, apperr.Conflict("task_already_accepted",
			fmt.Sprintf("Card nay dang o trang thai \"%s\", khong can nhan lai", assignStatusLabel[a.Status]))
	}

	quota := strings.TrimSpace(in.Quota)
	if quota == "" {
		quota = t.Quota
	}
	if quota == "" {
		return nil, apperr.Invalid("accept_requires_quota", "Phai chon dinh muc truoc khi nhan task")
	}
	qty := in.Quantity
	if qty <= 0 {
		return nil, apperr.Invalid("accept_requires_quantity", "Phai chon so luong (> 0) truoc khi nhan task")
	}
	if qty > t.Quantity {
		return nil, apperr.Invalid("accept_quantity_too_large",
			fmt.Sprintf("So luong nhan (%d) vuot qua so luong yeu cau (%d)", qty, t.Quantity)).
			WithDetail("task_quantity", t.Quantity)
	}

	now := time.Now().UTC()
	return s.applyTransition(ctx, t, a, in.ActorID, domain.AssignInProgress, map[string]any{
		"accepted_at": now,
		"quota":       quota,
		"quantity":    qty,
		"note":        in.Note,
	}, domain.EventTaskAccepted, func(name string) string {
		return fmt.Sprintf("%s da nhan %s (dinh muc: %s, so luong: %d)", name, t.Code, quota, qty)
	}, domain.JSONMap{"quota": quota, "quantity": qty})
}

// Pause — tam dung.
func (s *Service) Pause(ctx context.Context, in ActionInput) (*domain.Task, error) {
	t, a, err := s.loadForAction(ctx, in)
	if err != nil {
		return nil, err
	}
	return s.applyTransition(ctx, t, a, in.ActorID, domain.AssignPaused,
		map[string]any{"paused_at": time.Now().UTC()},
		domain.EventTaskPaused, func(name string) string {
			return withNote(fmt.Sprintf("%s tam dung %s", name, t.Code), in.Note)
		}, domain.JSONMap{"note": in.Note})
}

// Resume — tiep tuc (tu tam dung hoac treo).
func (s *Service) Resume(ctx context.Context, in ActionInput) (*domain.Task, error) {
	t, a, err := s.loadForAction(ctx, in)
	if err != nil {
		return nil, err
	}
	return s.applyTransition(ctx, t, a, in.ActorID, domain.AssignInProgress,
		map[string]any{"paused_at": nil, "held_at": nil},
		domain.EventTaskResumed, func(name string) string {
			return withNote(fmt.Sprintf("%s tiep tuc %s", name, t.Code), in.Note)
		}, nil)
}

// Hold — treo (cho dau vao tu nguoi khac).
func (s *Service) Hold(ctx context.Context, in ActionInput) (*domain.Task, error) {
	t, a, err := s.loadForAction(ctx, in)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.Note) == "" {
		return nil, apperr.Invalid("hold_requires_note", "Treo task phai ghi ly do")
	}
	return s.applyTransition(ctx, t, a, in.ActorID, domain.AssignOnHold,
		map[string]any{"held_at": time.Now().UTC()},
		domain.EventTaskHeld, func(name string) string {
			return fmt.Sprintf("%s treo %s: %s", name, t.Code, in.Note)
		}, domain.JSONMap{"note": in.Note})
}

// Complete — hoan thanh card. Con phieu sua dang mo thi khong cho hoan thanh.
func (s *Service) Complete(ctx context.Context, in ActionInput) (*domain.Task, error) {
	t, a, err := s.loadForAction(ctx, in)
	if err != nil {
		return nil, err
	}
	if open := countOpenRevisions(t, a.ID); open > 0 {
		return nil, apperr.Conflict("task_has_open_revision",
			fmt.Sprintf("Con %d phieu sua chua hoan thanh", open)).WithDetail("open_revisions", open)
	}
	return s.applyTransition(ctx, t, a, in.ActorID, domain.AssignCompleted,
		map[string]any{"completed_at": time.Now().UTC()},
		domain.EventTaskCompleted, func(name string) string {
			return withNote(fmt.Sprintf("%s hoan thanh %s", name, t.Code), in.Note)
		}, nil)
}

// ---------------- chia task / doi phan cong ----------------

type AssignInput struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
	UserIDs []uuid.UUID
	Role    domain.AssignmentRole
	Note    string
}

// AddAssignees — chia task cho nhieu nguoi (moi nguoi 1 card rieng) hoac goi ho tro/edit.
func (s *Service) AddAssignees(ctx context.Context, in AssignInput) (*domain.Task, error) {
	t, err := s.store.Tasks().GetByID(ctx, in.TaskID)
	if err != nil {
		return nil, err
	}
	if err := s.requireManager(ctx, t, in.ActorID); err != nil {
		return nil, err
	}
	role := in.Role
	if role == "" {
		role = domain.AssignPrimary
	}
	if !role.Valid() {
		return nil, apperr.Invalid("invalid_assignment_role", "role phai la primary|support|edit")
	}

	userIDs := uniqueIDs(in.UserIDs)
	if len(userIDs) == 0 {
		return nil, apperr.Invalid("assignees_required", "Phai co it nhat 1 nguoi nhan")
	}
	var fresh []uuid.UUID
	for _, uid := range userIDs {
		if t.AssignmentOf(uid) != nil {
			continue // da co card dang hieu luc
		}
		fresh = append(fresh, uid)
	}
	if len(fresh) == 0 {
		return nil, apperr.Conflict("already_assigned", "Nhung nguoi nay da co card tren task")
	}
	if err := s.ensureInConversation(ctx, t.ConversationID, fresh); err != nil {
		return nil, err
	}

	items := make([]domain.TaskAssignment, 0, len(fresh))
	for _, uid := range fresh {
		items = append(items, domain.TaskAssignment{
			ID: uuid.New(), TaskID: t.ID, UserID: uid, Role: role,
			Status: domain.AssignPending, AssignedBy: &in.ActorID, Note: in.Note,
		})
	}
	if err := s.store.Tasks().CreateAssignments(ctx, items); err != nil {
		return nil, err
	}
	if _, err := s.syncStatus(ctx, t); err != nil {
		return nil, err
	}

	names, err := s.userNames(ctx, fresh)
	if err != nil {
		return nil, err
	}
	actor, _ := s.userName(ctx, in.ActorID)
	s.notify(ctx, notifyInput{
		Task: t, ActorID: in.ActorID, EventType: domain.EventTaskAssigned,
		ToStatus: string(t.Status),
		Message: fmt.Sprintf("%s giao %s cho %s (%s)",
			actor, t.Code, strings.Join(names, ", "), assignRoleLabel[role]),
		Payload: domain.JSONMap{"user_ids": idStrings(fresh), "role": string(role)},
	})
	return s.store.Tasks().GetByID(ctx, t.ID)
}

type ReassignInput struct {
	ActorID      uuid.UUID
	TaskID       uuid.UUID
	AssignmentID *uuid.UUID // card can doi; nil => card primary dau tien
	ToUserID     uuid.UUID
	Note         string
}

// Reassign — admin doi nguoi phu trach: thu hoi card cu, tao card moi (cho nhan).
func (s *Service) Reassign(ctx context.Context, in ReassignInput) (*domain.Task, error) {
	t, err := s.store.Tasks().GetByID(ctx, in.TaskID)
	if err != nil {
		return nil, err
	}
	if err := s.requireManager(ctx, t, in.ActorID); err != nil {
		return nil, err
	}
	if in.ToUserID == uuid.Nil {
		return nil, apperr.Invalid("to_user_required", "Thieu nguoi phu trach moi")
	}

	old, err := s.pickAssignment(t, in.AssignmentID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	if old.UserID == in.ToUserID {
		return nil, apperr.Conflict("same_assignee", "Nguoi phu trach moi trung voi nguoi hien tai")
	}
	if t.AssignmentOf(in.ToUserID) != nil {
		return nil, apperr.Conflict("already_assigned", "Nguoi nay da co card tren task")
	}
	if err := s.ensureInConversation(ctx, t.ConversationID, []uuid.UUID{in.ToUserID}); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	newAssignment := domain.TaskAssignment{
		ID: uuid.New(), TaskID: t.ID, UserID: in.ToUserID, Role: old.Role,
		Status: domain.AssignPending, AssignedBy: &in.ActorID, Note: in.Note,
	}
	err = s.store.Tx().Do(ctx, func(ctx context.Context) error {
		if err := s.store.Tasks().UpdateAssignment(ctx, old.ID, map[string]any{
			"status": domain.AssignRevoked, "revoked_at": now,
		}); err != nil {
			return err
		}
		return s.store.Tasks().CreateAssignments(ctx, []domain.TaskAssignment{newAssignment})
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.syncStatus(ctx, t); err != nil {
		return nil, err
	}

	oldName := displayName(old)
	newName, _ := s.userName(ctx, in.ToUserID)
	actor, _ := s.userName(ctx, in.ActorID)
	s.notify(ctx, notifyInput{
		Task: t, ActorID: in.ActorID, EventType: domain.EventTaskReassigned,
		AssignmentID: &newAssignment.ID, ToStatus: string(t.Status),
		Message: withNote(fmt.Sprintf("%s doi nguoi phu trach %s: %s -> %s",
			actor, t.Code, oldName, newName), in.Note),
		Payload: domain.JSONMap{
			"from_user_id":      old.UserID.String(),
			"to_user_id":        in.ToUserID.String(),
			"old_assignment_id": old.ID.String(),
			"new_assignment_id": newAssignment.ID.String(),
		},
	})
	return s.store.Tasks().GetByID(ctx, t.ID)
}

// ---------------- dieu chinh thong tin / huy ----------------

type UpdateInput struct {
	ActorID  uuid.UUID
	TaskID   uuid.UUID
	Title    *string
	Desc     *string
	TeamID   *uuid.UUID
	TeamCode string
	Quantity *int
	Quota    *string
	Deadline *time.Time
	Priority *domain.TaskPriority
	// ClearDeadline = true: xoa deadline (vi Deadline nil nghia la "khong doi").
	ClearDeadline bool
}

// Update — admin dieu chinh thong tin cong viec, doi to.
func (s *Service) Update(ctx context.Context, in UpdateInput) (*domain.Task, error) {
	t, err := s.store.Tasks().GetByID(ctx, in.TaskID)
	if err != nil {
		return nil, err
	}
	if err := s.requireManager(ctx, t, in.ActorID); err != nil {
		return nil, err
	}

	fields := map[string]any{}
	changes := []string{}

	if in.Title != nil && strings.TrimSpace(*in.Title) != "" && *in.Title != t.Title {
		fields["title"] = strings.TrimSpace(*in.Title)
		changes = append(changes, "tieu de")
	}
	if in.Desc != nil && *in.Desc != t.Description {
		fields["description"] = *in.Desc
		changes = append(changes, "mo ta")
	}
	if in.TeamID != nil || in.TeamCode != "" {
		team, err := s.resolveTeam(ctx, in.TeamID, in.TeamCode)
		if err != nil {
			return nil, err
		}
		if team != nil && (t.TeamID == nil || *t.TeamID != team.ID) {
			fields["team_id"] = team.ID
			changes = append(changes, "to -> "+team.Name)
		}
	}
	if in.Quantity != nil {
		if *in.Quantity <= 0 {
			return nil, apperr.Invalid("invalid_quantity", "So luong phai > 0")
		}
		if *in.Quantity != t.Quantity {
			fields["quantity"] = *in.Quantity
			changes = append(changes, fmt.Sprintf("so luong -> %d", *in.Quantity))
		}
	}
	if in.Quota != nil && *in.Quota != t.Quota {
		fields["quota"] = *in.Quota
		changes = append(changes, "dinh muc -> "+*in.Quota)
	}
	if in.ClearDeadline {
		fields["deadline"] = nil
		changes = append(changes, "bo deadline")
	} else if in.Deadline != nil {
		fields["deadline"] = *in.Deadline
		changes = append(changes, "deadline -> "+in.Deadline.Format("02/01/2006 15:04"))
	}
	if in.Priority != nil {
		if !in.Priority.Valid() {
			return nil, apperr.Invalid("invalid_priority", "priority phai la low|normal|high|urgent")
		}
		if *in.Priority != t.Priority {
			fields["priority"] = *in.Priority
			changes = append(changes, "uu tien -> "+string(*in.Priority))
		}
	}

	if len(fields) == 0 {
		return t, nil
	}
	if err := s.store.Tasks().UpdateTask(ctx, t.ID, fields); err != nil {
		return nil, err
	}

	updated, err := s.store.Tasks().GetByID(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	actor, _ := s.userName(ctx, in.ActorID)
	s.notify(ctx, notifyInput{
		Task: updated, ActorID: in.ActorID, EventType: domain.EventTaskUpdated,
		ToStatus: string(updated.Status),
		Message:  fmt.Sprintf("%s cap nhat %s: %s", actor, updated.Code, strings.Join(changes, ", ")),
		Payload:  domain.JSONMap{"changes": changes},
	})
	return updated, nil
}

// Cancel — huy task, thu hoi cac card chua hoan thanh.
func (s *Service) Cancel(ctx context.Context, actorID, taskID uuid.UUID, reason string) (*domain.Task, error) {
	t, err := s.store.Tasks().GetByID(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := s.requireManager(ctx, t, actorID); err != nil {
		return nil, err
	}
	if t.Status == domain.TaskCancelled {
		return t, nil
	}

	now := time.Now().UTC()
	err = s.store.Tx().Do(ctx, func(ctx context.Context) error {
		for _, a := range t.ActiveAssignments() {
			if a.Status == domain.AssignCompleted {
				continue
			}
			if err := s.store.Tasks().UpdateAssignment(ctx, a.ID, map[string]any{
				"status": domain.AssignRevoked, "revoked_at": now,
			}); err != nil {
				return err
			}
		}
		return s.store.Tasks().UpdateTask(ctx, t.ID, map[string]any{"status": domain.TaskCancelled})
	})
	if err != nil {
		return nil, err
	}
	t.Status = domain.TaskCancelled

	actor, _ := s.userName(ctx, actorID)
	s.notify(ctx, notifyInput{
		Task: t, ActorID: actorID, EventType: domain.EventTaskCancelled,
		ToStatus: string(domain.TaskCancelled),
		Message:  withNote(fmt.Sprintf("%s huy %s", actor, t.Code), reason),
		Payload:  domain.JSONMap{"reason": reason},
	})
	return s.store.Tasks().GetByID(ctx, t.ID)
}

// ---------------- noi bo ----------------

// loadForAction: lay task + card can tac dong + kiem tra quyen.
func (s *Service) loadForAction(ctx context.Context, in ActionInput) (*domain.Task, *domain.TaskAssignment, error) {
	t, err := s.store.Tasks().GetByID(ctx, in.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, t.ConversationID, in.ActorID); err != nil {
		return nil, nil, err
	}
	if t.Status == domain.TaskCancelled {
		return nil, nil, apperr.Conflict("task_cancelled", "Task da bi huy")
	}

	a, err := s.pickAssignment(t, in.AssignmentID, in.ActorID)
	if err != nil {
		return nil, nil, err
	}
	// Card cua minh thi tu lam; card cua nguoi khac phai la admin/nguoi tao.
	if a.UserID != in.ActorID {
		if err := s.requireManager(ctx, t, in.ActorID); err != nil {
			return nil, nil, err
		}
	}
	return t, a, nil
}

// pickAssignment: theo id, hoac card cua actor, hoac card primary dau tien.
func (s *Service) pickAssignment(t *domain.Task, id *uuid.UUID, actorID uuid.UUID) (*domain.TaskAssignment, error) {
	if id != nil {
		for i := range t.Assignments {
			if t.Assignments[i].ID == *id {
				if t.Assignments[i].Status == domain.AssignRevoked {
					return nil, apperr.Conflict("assignment_revoked", "Card nay da bi thu hoi")
				}
				return &t.Assignments[i], nil
			}
		}
		return nil, apperr.ErrAssignmentNotFound
	}
	if actorID != uuid.Nil {
		if a := t.AssignmentOf(actorID); a != nil {
			return a, nil
		}
		return nil, apperr.Forbidden("no_assignment", "Ban khong duoc giao task nay")
	}
	for i := range t.Assignments {
		if t.Assignments[i].Status != domain.AssignRevoked &&
			t.Assignments[i].Role == domain.AssignPrimary {
			return &t.Assignments[i], nil
		}
	}
	return nil, apperr.ErrAssignmentNotFound
}

// applyTransition kiem tra state machine, ghi DB, suy lai trang thai task va thong bao.
func (s *Service) applyTransition(
	ctx context.Context,
	t *domain.Task,
	a *domain.TaskAssignment,
	actorID uuid.UUID,
	to domain.AssignmentStatus,
	fields map[string]any,
	evType domain.TaskEventType,
	msg func(actorName string) string,
	payload domain.JSONMap,
) (*domain.Task, error) {
	from := a.Status
	if !from.CanMoveTo(to) {
		return nil, apperr.Conflict("invalid_task_transition",
			fmt.Sprintf("Khong the chuyen tu \"%s\" sang \"%s\"",
				assignStatusLabel[from], assignStatusLabel[to])).
			WithDetail("from", string(from)).
			WithDetail("to", string(to)).
			WithDetail("allowed", domain.AllowedAssignmentTransitions[from])
	}

	fields["status"] = to
	if err := s.store.Tasks().UpdateAssignment(ctx, a.ID, fields); err != nil {
		return nil, err
	}
	if _, err := s.syncStatus(ctx, t); err != nil {
		return nil, err
	}

	name := displayName(a)
	if a.UserID != actorID {
		actorName, _ := s.userName(ctx, actorID)
		name = fmt.Sprintf("%s (thay %s)", actorName, name)
	}

	s.notify(ctx, notifyInput{
		Task: t, ActorID: actorID, EventType: evType,
		AssignmentID: &a.ID,
		FromStatus:   string(from), ToStatus: string(to),
		Message: msg(name),
		Payload: payload,
	})
	return s.store.Tasks().GetByID(ctx, t.ID)
}

func (s *Service) userName(ctx context.Context, id uuid.UUID) (string, error) {
	u, err := s.store.Users().GetByID(ctx, id)
	if err != nil {
		return id.String()[:8], err
	}
	if u.DisplayName == "" {
		return u.ExternalID, nil
	}
	return u.DisplayName, nil
}

func (s *Service) userNames(ctx context.Context, ids []uuid.UUID) ([]string, error) {
	users, err := s.store.Users().ListByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]string, len(users))
	for _, u := range users {
		byID[u.ID] = u.DisplayName
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok && n != "" {
			out = append(out, n)
			continue
		}
		out = append(out, id.String()[:8])
	}
	return out, nil
}

func countOpenRevisions(t *domain.Task, assignmentID uuid.UUID) int {
	n := 0
	for _, r := range t.Revisions {
		if r.Status != domain.RevisionOpen {
			continue
		}
		// Phieu khong gan card cu the thi tinh cho moi card.
		if r.AssignmentID == nil || *r.AssignmentID == assignmentID {
			n++
		}
	}
	return n
}

func withNote(base, note string) string {
	if strings.TrimSpace(note) == "" {
		return base
	}
	return base + ": " + note
}
