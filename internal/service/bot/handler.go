package bot

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// Request la input chuan hoa cho mot handler bot.
type Request struct {
	Bot            *domain.Bot
	TemplateCode   string
	Payload        domain.JSONMap
	RawText        string
	UserID         uuid.UUID
	ConversationID uuid.UUID
	MessageID      *uuid.UUID
	ThreadID       *uuid.UUID
}

// Result la output: tin nhan tra loi cua bot + submission da luu (neu co).
type Result struct {
	Submission *domain.BotSubmission
	ReplyText  string
	ReplyMeta  domain.JSONMap
	// Silent = true: khong tao tin nhan tra loi trong hoi thoai.
	Silent bool
}

// Handler: moi bot la 1 implement. Them bot moi = them file + Register().
type Handler interface {
	Handle(ctx context.Context, req *Request) (*Result, error)
}

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]Handler
	fallback Handler
}

func NewRegistry(fallback Handler) *Registry {
	return &Registry{handlers: map[string]Handler{}, fallback: fallback}
}

func (r *Registry) Register(slug string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[slug] = h
}

// Lookup: handler rieng theo slug, neu khong co thi dung fallback (TemplateHandler).
func (r *Registry) Lookup(slug string) (Handler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if h, ok := r.handlers[slug]; ok {
		return h, nil
	}
	if r.fallback != nil {
		return r.fallback, nil
	}
	return nil, fmt.Errorf("bot: khong co handler cho @%s", slug)
}
