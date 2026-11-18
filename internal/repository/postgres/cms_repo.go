package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// ---------------- nhom do CMS quan ly ----------------

func (r *ConversationRepo) GetByExternalID(ctx context.Context, externalID string) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.db(ctx).
		Preload("Members", func(db *gorm.DB) *gorm.DB { return db.Where("left_at IS NULL") }).
		Preload("Members.User").
		First(&c, "external_id = ?", externalID).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrConvNotFound)
	}
	return &c, nil
}

func (r *ConversationRepo) UpsertFromCMS(ctx context.Context, in *domain.Conversation) (*domain.Conversation, error) {
	if in.ExternalID == nil || *in.ExternalID == "" {
		return nil, apperr.Invalid("missing_external_id", "Nhom CMS phai co chatId")
	}

	var existing domain.Conversation
	err := r.db(ctx).First(&existing, "external_id = ?", *in.ExternalID).Error
	now := time.Now().UTC()

	switch {
	case err == nil:
		fields := map[string]any{"synced_at": now, "source": domain.SourceCMS}
		if in.Name != "" {
			fields["name"] = in.Name
		}
		if in.CMSInfo != nil {
			fields["cms_info"] = in.CMSInfo
		}
		if uerr := r.db(ctx).Model(&domain.Conversation{}).
			Where("id = ?", existing.ID).Updates(fields).Error; uerr != nil {
			return nil, wrap(uerr, "conversation.upsertFromCMS.update")
		}
		return r.GetByID(ctx, existing.ID)

	case errors.Is(err, gorm.ErrRecordNotFound):
		if in.ID == uuid.Nil {
			in.ID = uuid.New()
		}
		in.Source = domain.SourceCMS
		in.SyncedAt = &now
		if in.Type == "" {
			in.Type = domain.ConversationGroup
		}
		if cerr := r.db(ctx).Omit("Members").Create(in).Error; cerr != nil {
			return nil, wrap(cerr, "conversation.upsertFromCMS.create")
		}
		return r.GetByID(ctx, in.ID)

	default:
		return nil, wrap(err, "conversation.upsertFromCMS.lookup")
	}
}

// ReplaceMembers dat danh sach thanh vien = dung danh sach CMS tra ve.
// Nguoi khong con trong danh sach bi set left_at (mat quyen xem nhom ngay).
func (r *ConversationRepo) ReplaceMembers(ctx context.Context, convID uuid.UUID, members []domain.ConversationMember) error {
	now := time.Now().UTC()
	keep := make([]uuid.UUID, 0, len(members))
	for i := range members {
		if members[i].ID == uuid.Nil {
			members[i].ID = uuid.New()
		}
		members[i].ConversationID = convID
		if members[i].Role == "" {
			members[i].Role = domain.MemberNormal
		}
		if members[i].JoinedAt.IsZero() {
			members[i].JoinedAt = now
		}
		keep = append(keep, members[i].UserID)
	}

	return r.db(ctx).Transaction(func(tx *gorm.DB) error {
		if len(members) > 0 {
			err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{
					"role", "sources", "team_codes", "left_at",
				}),
			}).Create(&members).Error
			if err != nil {
				return wrap(err, "conversation.replaceMembers.upsert")
			}
		}

		q := tx.Model(&domain.ConversationMember{}).
			Where("conversation_id = ? AND left_at IS NULL", convID)
		if len(keep) > 0 {
			q = q.Where("user_id NOT IN ?", keep)
		}
		return wrap(q.Update("left_at", now).Error, "conversation.replaceMembers.prune")
	})
}

// ---------------- card ----------------

// UpdateCard chi ghi de khi version moi lon hon (chong ghi nguoc khi event den sai thu tu).
func (r *MessageRepo) UpdateCard(ctx context.Context, messageID uuid.UUID, card domain.JSONMap, version int) (bool, error) {
	res := r.db(ctx).Model(&domain.Message{}).
		Where("id = ? AND card_version < ?", messageID, version).
		Updates(map[string]any{"card": card, "card_version": version, "updated_at": time.Now().UTC()})
	if res.Error != nil {
		return false, wrap(res.Error, "message.updateCard")
	}
	return res.RowsAffected > 0, nil
}

// ---------------- notifications ----------------

type NotificationRepo struct{ base }

var _ repository.NotificationRepository = (*NotificationRepo)(nil)

func (r *NotificationRepo) Create(ctx context.Context, n *domain.Notification) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	return wrap(r.db(ctx).Create(n).Error, "notification.create")
}

func (r *NotificationRepo) ListByUser(ctx context.Context, userID uuid.UUID, p paging.Page) ([]domain.Notification, error) {
	q := r.db(ctx).Model(&domain.Notification{}).Where("user_id = ?", userID)
	if p.Cursor.Valid {
		q = q.Where("(created_at, id) < (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}
	var out []domain.Notification
	err := q.Order("created_at DESC, id DESC").Limit(p.FetchLimit()).Find(&out).Error
	if err != nil {
		return nil, wrap(err, "notification.listByUser")
	}
	return out, nil
}

func (r *NotificationRepo) MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, all bool) (int64, error) {
	q := r.db(ctx).Model(&domain.Notification{}).Where("user_id = ? AND NOT read", userID)
	if !all {
		if len(ids) == 0 {
			return 0, nil
		}
		q = q.Where("id IN ?", ids)
	}
	res := q.Updates(map[string]any{"read": true, "read_at": time.Now().UTC()})
	if res.Error != nil {
		return 0, wrap(res.Error, "notification.markRead")
	}
	return res.RowsAffected, nil
}

func (r *NotificationRepo) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.db(ctx).Model(&domain.Notification{}).
		Where("user_id = ? AND NOT read", userID).Count(&n).Error
	if err != nil {
		return 0, wrap(err, "notification.countUnread")
	}
	return n, nil
}

// ---------------- idempotency ----------------

type IdempotencyRepo struct{ base }

var _ repository.IdempotencyRepository = (*IdempotencyRepo)(nil)

func (r *IdempotencyRepo) Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	var rec domain.IdempotencyRecord
	err := r.db(ctx).First(&rec, "key = ?", key).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, wrap(err, "idempotency.get")
	}
	return &rec, nil
}

func (r *IdempotencyRepo) Save(ctx context.Context, rec *domain.IdempotencyRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	err := r.db(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(rec).Error
	return wrap(err, "idempotency.save")
}

// ---------------- outbox ----------------

type OutboxRepo struct{ base }

var _ repository.OutboxRepository = (*OutboxRepo)(nil)

func (r *OutboxRepo) Enqueue(ctx context.Context, e *domain.OutboxEvent) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.NextRetryAt.IsZero() {
		e.NextRetryAt = time.Now().UTC()
	}
	if e.Status == "" {
		e.Status = "pending"
	}
	err := r.db(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "event_id"}}, DoNothing: true}).
		Create(e).Error
	return wrap(err, "outbox.enqueue")
}

func (r *OutboxRepo) PickPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []domain.OutboxEvent
	err := r.db(ctx).
		Where("status = ? AND next_retry_at <= ?", "pending", time.Now().UTC()).
		Order("next_retry_at ASC").Limit(limit).Find(&out).Error
	if err != nil {
		return nil, wrap(err, "outbox.pickPending")
	}
	return out, nil
}

func (r *OutboxRepo) MarkSent(ctx context.Context, id uuid.UUID) error {
	now := time.Now().UTC()
	err := r.db(ctx).Model(&domain.OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]any{"status": "sent", "sent_at": now, "last_error": ""}).Error
	return wrap(err, "outbox.markSent")
}

func (r *OutboxRepo) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, nextRetry time.Time, giveUp bool) error {
	fields := map[string]any{
		"last_error":    errMsg,
		"next_retry_at": nextRetry,
		"attempts":      gorm.Expr("attempts + 1"),
	}
	if giveUp {
		fields["status"] = "failed"
	}
	err := r.db(ctx).Model(&domain.OutboxEvent{}).Where("id = ?", id).Updates(fields).Error
	return wrap(err, "outbox.markFailed")
}
