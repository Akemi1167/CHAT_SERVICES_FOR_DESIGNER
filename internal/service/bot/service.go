package bot

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// Service la cua ngo duy nhat cho moi thao tac lien quan bot.
// Luu y: Service KHONG tao tin nhan — viec do thuoc message service
// (tranh phu thuoc vong giua 2 package).
type Service struct {
	store    repository.Store
	registry *Registry
}

func New(store repository.Store) *Service {
	return &Service{
		store:    store,
		registry: NewRegistry(NewTemplateHandler(store)),
	}
}

// Register cho phep gan handler rieng cho 1 slug (vd "@ai" goi sang AI service).
func (s *Service) Register(slug string, h Handler) { s.registry.Register(slug, h) }

type DispatchInput struct {
	// Mot trong 2: Content ("@bot yeu_cau ...") hoac (Slug + TemplateCode + Payload).
	Content      string
	Slug         string
	TemplateCode string
	Payload      domain.JSONMap

	UserID         uuid.UUID
	ConversationID uuid.UUID
	MessageID      *uuid.UUID
	ThreadID       *uuid.UUID
}

// Detect kiem tra noi dung co goi bot hay khong (de set message type = bot_command).
func Detect(content string) (string, bool) {
	m, ok := ParseMention(content)
	if !ok {
		return "", false
	}
	return m.Slug, true
}

// Dispatch chay handler cua bot. Tra (nil, nil) neu noi dung khong goi bot nao.
func (s *Service) Dispatch(ctx context.Context, in DispatchInput) (*Result, error) {
	slug := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(in.Slug), "@"))
	code := in.TemplateCode
	payload := in.Payload
	raw := in.Content

	// Luon thu parse noi dung "@bot <ma_mau> ...".
	// Gia tri truyen tay (Slug/TemplateCode/Payload tu form FE) co uu tien cao hon.
	if m, ok := ParseMention(in.Content); ok {
		if slug == "" {
			slug = m.Slug
		}
		parsedCode, parsedPayload, _ := ParseBody(m.Body)
		if code == "" {
			code = parsedCode
		}
		if len(payload) == 0 {
			payload = parsedPayload
		}
	}

	// Khong goi bot nao.
	if slug == "" {
		return nil, nil
	}

	b, err := s.store.Bots().GetBotBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	h, err := s.registry.Lookup(slug)
	if err != nil {
		return nil, apperr.Internal("bot_handler_missing", "Bot chua co handler").Wrap(err)
	}

	return h.Handle(ctx, &Request{
		Bot:            b,
		TemplateCode:   strings.ToLower(strings.TrimSpace(code)),
		Payload:        payload,
		RawText:        raw,
		UserID:         in.UserID,
		ConversationID: in.ConversationID,
		MessageID:      in.MessageID,
		ThreadID:       in.ThreadID,
	})
}

func (s *Service) ListBots(ctx context.Context) ([]domain.Bot, error) {
	return s.store.Bots().ListBots(ctx)
}

func (s *Service) ListTemplates(ctx context.Context, slug string) (*domain.Bot, []domain.BotTemplate, error) {
	b, err := s.store.Bots().GetBotBySlug(ctx, strings.TrimPrefix(slug, "@"))
	if err != nil {
		return nil, nil, err
	}
	tpls, err := s.store.Bots().ListTemplates(ctx, b.ID)
	if err != nil {
		return nil, nil, err
	}
	return b, tpls, nil
}

type ListSubmissionsInput struct {
	BotSlug        string
	ConversationID *uuid.UUID
	Status         *domain.SubmissionStatus
}

func (s *Service) ListSubmissions(ctx context.Context, in ListSubmissionsInput, p paging.Page) ([]domain.BotSubmission, paging.PageInfo, error) {
	f := repository.SubmissionFilter{ConversationID: in.ConversationID, Status: in.Status}
	if slug := strings.TrimPrefix(strings.TrimSpace(in.BotSlug), "@"); slug != "" {
		b, err := s.store.Bots().GetBotBySlug(ctx, slug)
		if err != nil {
			return nil, paging.PageInfo{}, err
		}
		f.BotID = &b.ID
	}
	rows, err := s.store.Bots().ListSubmissions(ctx, f, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, func(s domain.BotSubmission) (time.Time, string) {
		return s.CreatedAt, s.ID.String()
	})
	return items, info, nil
}
