// Package notification quan ly thong bao do CMS day sang (Bot API /notifications).
package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type Service struct {
	store repository.Store
	hub   *realtime.Hub
}

func New(store repository.Store, hub *realtime.Hub) *Service {
	return &Service{store: store, hub: hub}
}

type CreateInput struct {
	UserID    uuid.UUID
	Text      string
	MessageID *uuid.UUID
	Payload   domain.JSONMap
}

// Create luu thong bao va day realtime ngay cho dung nguoi nhan.
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Notification, error) {
	n := &domain.Notification{
		ID:        uuid.New(),
		UserID:    in.UserID,
		Text:      in.Text,
		MessageID: in.MessageID,
		Payload:   in.Payload,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.store.Notifications().Create(ctx, n); err != nil {
		return nil, err
	}
	// Thieu vi tri thi FE van hien duoc thong bao, chi khong nhay toi tin duoc.
	if err := s.attachLocations(ctx, []*domain.Notification{n}); err != nil {
		slog.Warn("khong tra duoc vi tri tin cua thong bao", "notification_id", n.ID, "err", err)
	}
	s.hub.Publish(ctx, realtime.NewEvent(
		realtime.EventNotification, "", []uuid.UUID{in.UserID}, n,
	))
	return n, nil
}

// List tra thong bao cua userID. NotificationFilter zero = khong loc gi.
func (s *Service) List(ctx context.Context, userID uuid.UUID, f repository.NotificationFilter, p paging.Page) ([]domain.Notification, paging.PageInfo, error) {
	rows, err := s.store.Notifications().ListByUser(ctx, userID, f, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, func(n domain.Notification) (time.Time, string) {
		return n.CreatedAt, n.ID.String()
	})
	refs := make([]*domain.Notification, len(items))
	for i := range items {
		refs[i] = &items[i]
	}
	if err := s.attachLocations(ctx, refs); err != nil {
		return nil, paging.PageInfo{}, err
	}
	return items, info, nil
}

// attachLocations dien conversationId / groupId / threadRootId cho thong bao
// co messageId, bang MOT query cho ca trang. Tin da bi xoa cung thi de trong.
func (s *Service) attachLocations(ctx context.Context, items []*domain.Notification) error {
	ids := make([]uuid.UUID, 0, len(items))
	for _, n := range items {
		if n.MessageID != nil {
			ids = append(ids, *n.MessageID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	locs, err := s.store.Messages().Locations(ctx, ids)
	if err != nil {
		return err
	}
	for _, n := range items {
		if n.MessageID == nil {
			continue
		}
		loc, ok := locs[*n.MessageID]
		if !ok {
			continue
		}
		convID := loc.ConversationID
		n.ConversationID = &convID
		n.GroupID = loc.GroupID
		n.ThreadRootID = loc.ThreadID
	}
	return nil
}

// MarkRead danh dau da doc; all=true thi danh dau het.
func (s *Service) MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, all bool) (int64, error) {
	n, err := s.store.Notifications().MarkRead(ctx, userID, ids, all)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func (s *Service) CountUnread(ctx context.Context, userID uuid.UUID) (int64, error) {
	return s.store.Notifications().CountUnread(ctx, userID)
}
