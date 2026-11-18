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

// ---------------- phieu sua ----------------

type CreateRevisionInput struct {
	ActorID uuid.UUID
	TaskID  uuid.UUID
	// AssignmentID: phieu cho card cu the; nil => tinh cho card primary.
	AssignmentID *uuid.UUID
	Reason       string // nguyen nhan (bat buoc)
	RevisionType string // loai sua
	Quantity     int
	SubmissionID *uuid.UUID
	MessageID    *uuid.UUID
	Metadata     domain.JSONMap
}

// CreateRevision tao phieu sua. Neu card lien quan da hoan thanh thi mo lai
// (completed -> in_progress) de nguoi phu trach sua tiep.
func (s *Service) CreateRevision(ctx context.Context, in CreateRevisionInput) (*domain.TaskRevision, *domain.Task, error) {
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
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, nil, apperr.Invalid("revision_reason_required", "Phieu sua phai ghi nguyen nhan")
	}
	if strings.TrimSpace(in.RevisionType) == "" {
		return nil, nil, apperr.Invalid("revision_type_required", "Phieu sua phai chon loai sua")
	}
	qty := in.Quantity
	if qty <= 0 {
		qty = 1
	}

	// Xac dinh card bi anh huong (neu co).
	var target *domain.TaskAssignment
	if in.AssignmentID != nil {
		target, err = s.pickAssignment(t, in.AssignmentID, uuid.Nil)
		if err != nil {
			return nil, nil, err
		}
	} else if a, perr := s.pickAssignment(t, nil, uuid.Nil); perr == nil {
		target = a
	}

	rev := &domain.TaskRevision{
		ID:           uuid.New(),
		TaskID:       t.ID,
		Reason:       reason,
		RevisionType: strings.TrimSpace(in.RevisionType),
		Quantity:     qty,
		Status:       domain.RevisionOpen,
		SubmissionID: in.SubmissionID,
		MessageID:    in.MessageID,
		CreatedBy:    in.ActorID,
		Metadata:     in.Metadata,
	}
	if target != nil {
		rev.AssignmentID = &target.ID
	}

	err = s.store.Tx().Do(ctx, func(ctx context.Context) error {
		code, err := s.store.Tasks().NextRevisionCode(ctx)
		if err != nil {
			return err
		}
		rev.Code = code
		if err := s.store.Tasks().CreateRevision(ctx, rev); err != nil {
			return err
		}
		// Mo lai card da hoan thanh de sua.
		if target != nil && target.Status == domain.AssignCompleted {
			return s.store.Tasks().UpdateAssignment(ctx, target.ID, map[string]any{
				"status": domain.AssignInProgress, "completed_at": nil,
			})
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.syncStatus(ctx, t); err != nil {
		return nil, nil, err
	}

	actor, _ := s.userName(ctx, in.ActorID)
	who := ""
	if target != nil {
		who = " cho " + displayName(target)
	}
	s.notify(ctx, notifyInput{
		Task: t, ActorID: in.ActorID, EventType: domain.EventRevisionCreated,
		AssignmentID: rev.AssignmentID, RevisionID: &rev.ID, ToStatus: string(t.Status),
		Message: fmt.Sprintf("%s tao phieu sua %s%s · loai: %s · so luong: %d · nguyen nhan: %s",
			actor, rev.Code, who, rev.RevisionType, rev.Quantity, rev.Reason),
		Payload: domain.JSONMap{
			"revision_id":   rev.ID.String(),
			"revision_code": rev.Code,
			"revision_type": rev.RevisionType,
			"quantity":      rev.Quantity,
			"reason":        rev.Reason,
		},
	})

	full, err := s.store.Tasks().GetByID(ctx, t.ID)
	if err != nil {
		return rev, t, nil
	}
	return rev, full, nil
}

// CompleteRevision hoan thanh RIENG tung phieu sua.
func (s *Service) CompleteRevision(ctx context.Context, actorID, revisionID uuid.UUID, note string) (*domain.TaskRevision, *domain.Task, error) {
	rev, err := s.store.Tasks().GetRevision(ctx, revisionID)
	if err != nil {
		return nil, nil, err
	}
	t, err := s.store.Tasks().GetByID(ctx, rev.TaskID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, t.ConversationID, actorID); err != nil {
		return nil, nil, err
	}
	if rev.Status != domain.RevisionOpen {
		return nil, nil, apperr.Conflict("revision_not_open",
			fmt.Sprintf("Phieu %s da o trang thai %s", rev.Code, rev.Status))
	}

	// Nguoi sua (chu card) hoac admin/nguoi tao task moi duoc dong phieu.
	allowed := false
	if a := t.AssignmentOf(actorID); a != nil {
		allowed = rev.AssignmentID == nil || *rev.AssignmentID == a.ID
	}
	if !allowed {
		if err := s.requireManager(ctx, t, actorID); err != nil {
			return nil, nil, apperr.Forbidden("cannot_complete_revision",
				"Chi nguoi phu trach card hoac admin duoc hoan thanh phieu sua")
		}
	}

	now := time.Now().UTC()
	if err := s.store.Tasks().UpdateRevision(ctx, rev.ID, map[string]any{
		"status": domain.RevisionCompleted, "completed_by": actorID, "completed_at": now,
	}); err != nil {
		return nil, nil, err
	}
	rev.Status = domain.RevisionCompleted
	rev.CompletedBy = &actorID
	rev.CompletedAt = &now

	actor, _ := s.userName(ctx, actorID)
	remaining := 0
	for _, r := range t.Revisions {
		if r.ID != rev.ID && r.Status == domain.RevisionOpen {
			remaining++
		}
	}
	s.notify(ctx, notifyInput{
		Task: t, ActorID: actorID, EventType: domain.EventRevisionCompleted,
		AssignmentID: rev.AssignmentID, RevisionID: &rev.ID, ToStatus: string(t.Status),
		Message: withNote(fmt.Sprintf("%s hoan thanh phieu sua %s (con %d phieu dang mo)",
			actor, rev.Code, remaining), note),
		Payload: domain.JSONMap{
			"revision_id":         rev.ID.String(),
			"revision_code":       rev.Code,
			"open_revisions_left": remaining,
		},
	})

	full, err := s.store.Tasks().GetByID(ctx, t.ID)
	if err != nil {
		return rev, t, nil
	}
	return rev, full, nil
}

// ListRevisions tra toan bo phieu sua cua task.
func (s *Service) ListRevisions(ctx context.Context, actorID, taskID uuid.UUID) ([]domain.TaskRevision, error) {
	if _, err := s.Get(ctx, actorID, taskID); err != nil {
		return nil, err
	}
	return s.store.Tasks().ListRevisions(ctx, taskID)
}
