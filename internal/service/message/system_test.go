package message

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
)

// Store gia: chi cai dat dung cac ham PostSystem goi (goi ham khac se panic).
type sysStore struct{ repository.Store }
type sysMsgs struct{ repository.MessageRepository }
type sysConvs struct {
	repository.ConversationRepository
	members []uuid.UUID
}

func (s sysStore) Messages() repository.MessageRepository { return sysMsgs{} }
func (s sysStore) Conversations() repository.ConversationRepository {
	return sysConvs{members: []uuid.UUID{uuid.New(), uuid.New()}}
}
func (sysMsgs) Create(context.Context, *domain.Message) error { return nil }
func (sysConvs) UpdateLastMessage(context.Context, uuid.UUID, uuid.UUID, time.Time) error {
	return nil
}
func (c sysConvs) ListMemberUserIDs(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return c.members, nil
}

func TestPostSystemEventType(t *testing.T) {
	cases := []struct {
		name  string
		event realtime.EventType
		want  realtime.EventType
	}{
		{"mac dinh la message", "", realtime.EventMessageNew},
		{"dong lich su gui form", realtime.EventFormSubmitted, "form.submitted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			broker := realtime.NewMemoryBroker(8)
			events, err := broker.Subscribe(ctx)
			if err != nil {
				t.Fatal(err)
			}
			hub := realtime.NewHub(broker)
			store := sysStore{}
			svc := New(store, conversation.New(store, hub), nil, hub)

			msg, err := svc.PostSystem(ctx, SystemMessageInput{
				ConversationID: uuid.New(),
				Content:        "SKY · Đã gửi yêu cầu WEB-20261127-0009",
				Metadata:       domain.JSONMap{"kind": "form_submitted"},
				Event:          tc.event,
			})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case ev := <-events:
				if ev.Type != tc.want {
					t.Fatalf("type = %q, want %q", ev.Type, tc.want)
				}
				if got, _ := ev.Payload.(*domain.Message); got == nil || got.ID != msg.ID {
					t.Fatalf("payload khong phai tin vua tao: %#v", ev.Payload)
				}
				if len(ev.Recipients) != 2 {
					t.Fatalf("recipients = %v", ev.Recipients)
				}
			case <-ctx.Done():
				t.Fatal("khong nhan duoc event")
			}
		})
	}
}
