package realtime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// Qua Redis Pub/Sub, Payload bi JSON hoa thanh map[string]any. Neu khong doc lai
// kieu thi be mat client se dung sai hinh dang (giu "content" thay vi "text").
func TestRestorePayloadSauKhiQuaBroker(t *testing.T) {
	msg := &domain.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Type:           domain.MessageText,
		SenderType:     domain.SenderUser,
		Content:        "qua redis",
		CreatedAt:      time.Now().UTC(),
	}
	ev := NewEvent(EventMessageNew, msg.ConversationID.String(), []uuid.UUID{uuid.New()}, msg)

	// Mo phong vong Publish -> Subscribe cua RedisBroker.
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, isMap := got.Payload.(map[string]any); !isMap {
		t.Fatalf("truoc khi khoi phuc phai la map, nhan %T", got.Payload)
	}

	got.RestorePayload()

	m, ok := got.Payload.(*domain.Message)
	if !ok {
		t.Fatalf("sau khi khoi phuc phai la *domain.Message, nhan %T", got.Payload)
	}
	if m.Content != "qua redis" || m.ID != msg.ID {
		t.Errorf("du lieu lech: %+v", m)
	}
}

func TestRestorePayloadVoiNotification(t *testing.T) {
	n := &domain.Notification{ID: uuid.New(), UserID: uuid.New(), Text: "ting", CreatedAt: time.Now().UTC()}
	ev := NewEvent(EventNotification, "", []uuid.UUID{n.UserID}, n)

	raw, _ := json.Marshal(ev)
	var got Event
	_ = json.Unmarshal(raw, &got)
	got.RestorePayload()

	if _, ok := got.Payload.(*domain.Notification); !ok {
		t.Fatalf("muon *domain.Notification, nhan %T", got.Payload)
	}
}

// message.deleted mang payload dang map (chi co id) — khong duoc coi la Message.
func TestRestorePayloadGiuMapKhiKhongPhaiMessage(t *testing.T) {
	cases := []struct {
		name string
		t    EventType
		p    map[string]any
	}{
		{"message.deleted", EventMessageDeleted, map[string]any{"conversation_id": "c1", "message_id": "m1"}},
		{"read", EventMessageRead, map[string]any{"conversation_id": "c1", "user_id": "u1"}},
		{"presence", EventPresence, map[string]any{"actor_id": "u_6", "online": true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ev := NewEvent(c.t, "c1", []uuid.UUID{uuid.New()}, c.p)
			raw, _ := json.Marshal(ev)
			var got Event
			_ = json.Unmarshal(raw, &got)
			got.RestorePayload()

			if _, ok := got.Payload.(map[string]any); !ok {
				t.Fatalf("phai giu map, nhan %T", got.Payload)
			}
		})
	}
}

// Payload khong phai map (chua qua broker) thi de nguyen.
func TestRestorePayloadKhongDoiKhiConKieuGoc(t *testing.T) {
	msg := &domain.Message{ID: uuid.New(), Content: "local"}
	ev := NewEvent(EventMessageNew, "c1", []uuid.UUID{uuid.New()}, msg)
	ev.RestorePayload()

	if got, ok := ev.Payload.(*domain.Message); !ok || got != msg {
		t.Fatalf("phai giu nguyen con tro goc, nhan %T", ev.Payload)
	}
}
