// Package notification quan ly thong bao do CMS day sang (Bot API /notifications).
package notification

import (
	"context"
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
	s.hub.Publish(ctx, realtime.NewEvent(
		realtime.EventNotification, "", []uuid.UUID{in.UserID}, n,
	))
	return n, nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, p paging.Page) ([]domain.Notification, paging.PageInfo, error) {
	rows, err := s.store.Notifications().ListByUser(ctx, userID, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, func(n domain.Notification) (time.Time, string) {
		return n.CreatedAt, n.ID.String()
	})
	return items, info, nil
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
