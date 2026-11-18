// Package outbox gui cac event bat dong bo sang CMS (message.created, user.updated).
//
// Event dong bo (form.submit, card.action) KHONG di qua day: UI cho ket qua nen
// goi thang CMS. Con event bat dong bo thi ghi DB truoc roi worker gui lai cho den
// khi thanh cong (CMS chong trung theo eventId).
package outbox

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type Service struct {
	store    repository.Store
	client   *cms.Client
	interval time.Duration
	maxRetry int
	log      *slog.Logger
}

func New(store repository.Store, client *cms.Client, interval time.Duration, maxRetry int) *Service {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	if maxRetry <= 0 {
		maxRetry = 8
	}
	return &Service{
		store: store, client: client, interval: interval, maxRetry: maxRetry,
		log: slog.Default().With("component", "service.outbox"),
	}
}

// EnqueueMessageCreated ghi event message.created vao outbox.
// Goi sau khi tin nhan cua NGUOI DUNG da luu thanh cong.
func (s *Service) EnqueueMessageCreated(ctx context.Context, msg *domain.Message) {
	if !s.client.Enabled() || msg == nil {
		return
	}

	conv, err := s.store.Conversations().GetByID(ctx, msg.ConversationID)
	if err != nil {
		s.log.Error("outbox: khong lay duoc hoi thoai", "err", err)
		return
	}
	groupID := conv.ID.String()
	if conv.ExternalID != nil && *conv.ExternalID != "" {
		groupID = *conv.ExternalID
	}

	senderActor := ""
	if msg.SenderID != nil {
		if u, err := s.store.Users().GetByID(ctx, *msg.SenderID); err == nil {
			senderActor = u.ActorID
		}
	}

	message := map[string]any{"id": msg.ID.String(), "text": msg.Content}
	if msg.ThreadID != nil {
		message["threadRootId"] = msg.ThreadID.String()
	}

	fields := map[string]any{
		"message": message,
		"group":   map[string]any{"id": groupID},
		"sender":  map[string]any{"id": senderActor},
	}
	if conv.CMSInfo != nil {
		if id, ok := conv.CMSInfo["botId"].(float64); ok {
			fields["botId"] = fmt.Sprintf("b_%d", int64(id))
		}
	}
	payload := cms.BuildEvent(cms.EventMessageCreate, fields)

	eventID, _ := payload["eventId"].(string)
	err = s.store.Outbox().Enqueue(ctx, &domain.OutboxEvent{
		EventID:     eventID,
		EventType:   cms.EventMessageCreate,
		Payload:     payload,
		NextRetryAt: time.Now().UTC(),
	})
	if err != nil {
		s.log.Error("outbox: ghi event that bai", "err", err)
	}
}

// Run lam viec dinh ky: lay event pending roi gui sang CMS.
func (s *Service) Run(ctx context.Context) error {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			s.flush(ctx)
		}
	}
}

func (s *Service) flush(ctx context.Context) {
	events, err := s.store.Outbox().PickPending(ctx, 50)
	if err != nil {
		s.log.Error("outbox: doc hang doi that bai", "err", err)
		return
	}
	for _, ev := range events {
		if _, err := s.client.SendRawEvent(ctx, ev.Payload, 0); err != nil {
			attempts := ev.Attempts + 1
			giveUp := attempts >= s.maxRetry
			next := time.Now().UTC().Add(backoff(attempts))
			if e := s.store.Outbox().MarkFailed(ctx, ev.ID, err.Error(), next, giveUp); e != nil {
				s.log.Error("outbox: cap nhat that bai", "err", e)
			}
			if giveUp {
				s.log.Error("outbox: bo cuoc sau nhieu lan thu",
					"event_id", ev.EventID, "type", ev.EventType, "attempts", attempts, "err", err)
			}
			continue
		}
		if err := s.store.Outbox().MarkSent(ctx, ev.ID); err != nil {
			s.log.Error("outbox: danh dau da gui that bai", "err", err)
		}
	}
}

// backoff: 2^n giay, toi da 10 phut.
func backoff(attempts int) time.Duration {
	d := time.Duration(math.Pow(2, float64(attempts))) * time.Second
	if d > 10*time.Minute {
		return 10 * time.Minute
	}
	return d
}
