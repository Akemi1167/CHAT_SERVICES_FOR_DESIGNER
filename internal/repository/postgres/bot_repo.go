package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type BotRepo struct{ base }

var _ repository.BotRepository = (*BotRepo)(nil)

func (r *BotRepo) ListBots(ctx context.Context) ([]domain.Bot, error) {
	var out []domain.Bot
	err := r.db(ctx).Where("enabled = true").Order("slug").Find(&out).Error
	if err != nil {
		return nil, wrap(err, "bot.list")
	}
	return out, nil
}

func (r *BotRepo) GetBotBySlug(ctx context.Context, slug string) (*domain.Bot, error) {
	var b domain.Bot
	err := r.db(ctx).First(&b, "slug = ? AND enabled = true", slug).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrBotNotFound)
	}
	return &b, nil
}

func (r *BotRepo) ListTemplates(ctx context.Context, botID uuid.UUID) ([]domain.BotTemplate, error) {
	var out []domain.BotTemplate
	err := r.db(ctx).Where("bot_id = ? AND enabled = true", botID).Order("code").Find(&out).Error
	if err != nil {
		return nil, wrap(err, "bot.listTemplates")
	}
	return out, nil
}

func (r *BotRepo) GetTemplate(ctx context.Context, botID uuid.UUID, code string) (*domain.BotTemplate, error) {
	var t domain.BotTemplate
	err := r.db(ctx).First(&t, "bot_id = ? AND code = ? AND enabled = true", botID, code).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrTemplateNotFound)
	}
	return &t, nil
}

func (r *BotRepo) CreateSubmission(ctx context.Context, s *domain.BotSubmission) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Status == "" {
		s.Status = domain.SubmissionStored
	}
	return wrap(r.db(ctx).Create(s).Error, "bot.createSubmission")
}

func (r *BotRepo) UpdateSubmissionStatus(ctx context.Context, id uuid.UUID, status domain.SubmissionStatus, errMsg string) error {
	err := r.db(ctx).Model(&domain.BotSubmission{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status, "error": errMsg}).Error
	return wrap(err, "bot.updateSubmissionStatus")
}

func (r *BotRepo) ListSubmissions(ctx context.Context, f repository.SubmissionFilter, p paging.Page) ([]domain.BotSubmission, error) {
	q := r.db(ctx).Model(&domain.BotSubmission{})
	if f.BotID != nil {
		q = q.Where("bot_id = ?", *f.BotID)
	}
	if f.ConversationID != nil {
		q = q.Where("conversation_id = ?", *f.ConversationID)
	}
	if f.UserID != nil {
		q = q.Where("user_id = ?", *f.UserID)
	}
	if f.Status != nil {
		q = q.Where("status = ?", *f.Status)
	}
	if p.Cursor.Valid {
		q = q.Where("(created_at, id) < (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}

	var out []domain.BotSubmission
	err := q.Order("created_at DESC, id DESC").Limit(p.FetchLimit()).Find(&out).Error
	if err != nil {
		return nil, wrap(err, "bot.listSubmissions")
	}
	return out, nil
}
