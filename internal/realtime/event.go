// Package realtime lo viec day su kien xuong client dang ket noi (WebSocket).
// Luong: service -> Hub.Publish -> Broker (memory|redis) -> moi instance -> client local.
package realtime

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

type EventType string

// Ten event theo hop dong voi CMS (docs/10-cms-integration.md):
// message | read | notification | presence | user | reload.
// Cac event con lai la mo rong rieng cua Chat.
const (
	EventMessageNew     EventType = "message" // tin moi HOAC tin duoc cap nhat (FE upsert theo id)
	EventMessageUpdated EventType = "message"
	EventCardUpdated    EventType = "message" // card doi -> gui lai ca tin nhan da loc visibleTo
	EventMessageDeleted EventType = "message.deleted"
	EventMessageRead    EventType = "read"
	EventTyping         EventType = "typing"
	EventPresence       EventType = "presence"
	EventNotification   EventType = "notification"
	EventUserUpdated    EventType = "user"
	// reload: FE nen goi lai bootstrap/groups (vao nhom moi, bi go khoi nhom...).
	EventReload              EventType = "reload"
	EventConversationCreated EventType = "reload"
	EventConversationUpdated EventType = "reload"
	EventBotSubmissionStored EventType = "bot.submission.stored"

	// Task: FE chi can 2 event nay la du de ve lai card.
	EventTaskCreated EventType = "task.created"
	EventTaskUpdated EventType = "task.updated"
)

// Event la don vi duoc broadcast. Recipients = danh sach user can nhan.
// Tinh recipients o service layer (thuong la thanh vien conversation).
type Event struct {
	ID             string      `json:"id"`
	Type           EventType   `json:"type"`
	ConversationID string      `json:"conversation_id,omitempty"`
	Recipients     []uuid.UUID `json:"recipients"`
	Payload        any         `json:"payload,omitempty"`
	CreatedAt      time.Time   `json:"created_at"`

	// SkipUserID: bo qua nguoi tao ra event (vd nguoi gui da co message qua ACK).
	SkipUserID *uuid.UUID `json:"skip_user_id,omitempty"`
}

// RestorePayload dung lai kieu cu the cho Payload sau khi event di qua broker.
//
// Qua Redis Pub/Sub, Payload bi json.Unmarshal thanh map[string]any nen tang
// tren khong con biet day la domain.Message hay gi khac — frame cho be mat
// client se dung sai hinh dang. Ham nay doc lai theo ev.Type.
//
// Khong doc duoc thi giu nguyen map: van gui duoc frame, chi la dang tho.
func (e *Event) RestorePayload() {
	raw, ok := e.Payload.(map[string]any)
	if !ok {
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return
	}
	switch e.Type {
	case EventMessageNew, EventMessageDeleted:
		// message.deleted mang payload dang map (chi co id) nen phai kiem:
		// co "content" hoac "sender_type" moi la mot domain.Message day du.
		if _, hasContent := raw["content"]; !hasContent {
			if _, hasSender := raw["sender_type"]; !hasSender {
				return
			}
		}
		var m domain.Message
		if json.Unmarshal(b, &m) == nil {
			e.Payload = &m
		}
	case EventNotification:
		var n domain.Notification
		if json.Unmarshal(b, &n) == nil {
			e.Payload = &n
		}
	}
}

func NewEvent(t EventType, convID string, recipients []uuid.UUID, payload any) Event {
	return Event{
		ID:             uuid.NewString(),
		Type:           t,
		ConversationID: convID,
		Recipients:     recipients,
		Payload:        payload,
		CreatedAt:      time.Now().UTC(),
	}
}
