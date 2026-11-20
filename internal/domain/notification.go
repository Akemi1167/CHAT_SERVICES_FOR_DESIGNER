package domain

import (
	"time"

	"github.com/google/uuid"
)

// Notification do CMS day sang qua Bot API (POST /bot/v1/notifications).
// Chat luu va phat realtime cho dung nguoi nhan.
type Notification struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	UserID    uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`
	Text      string     `gorm:"type:text;not null" json:"text"`
	MessageID *uuid.UUID `gorm:"type:uuid" json:"message_id,omitempty"`
	Payload   JSONMap    `gorm:"type:jsonb" json:"payload,omitempty"`
	Read      bool       `gorm:"default:false" json:"read"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`

	// Vi tri cua tin ma thong bao tro toi — KHONG luu DB, dien luc doc tu chinh
	// tin nhan (xem notification.Service). FE can de mo dung nhom / dung thread:
	// card cua mot phan (chia task) nam trong thread cua card goc.
	ConversationID *uuid.UUID `gorm:"-" json:"conversation_id,omitempty"`
	GroupID        string     `gorm:"-" json:"group_id,omitempty"`       // chatId cua CMS (wg_...), rong neu nhom noi bo
	ThreadRootID   *uuid.UUID `gorm:"-" json:"thread_root_id,omitempty"` // nil = tin nam o timeline nhom
}

func (Notification) TableName() string { return "notifications" }

// IdempotencyRecord luu ket qua cu cua Bot API theo Idempotency-Key.
type IdempotencyRecord struct {
	Key        string    `gorm:"size:128;primaryKey" json:"key"`
	Endpoint   string    `gorm:"size:128;not null" json:"endpoint"`
	StatusCode int       `gorm:"not null" json:"status_code"`
	Response   JSONMap   `gorm:"type:jsonb" json:"response"`
	CreatedAt  time.Time `json:"created_at"`
}

func (IdempotencyRecord) TableName() string { return "idempotency_keys" }

// OutboxEvent: event gui CMS bat dong bo (message.created, user.updated).
// Gui dong bo (form.submit, card.action) khong di qua bang nay.
type OutboxEvent struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	EventID     string     `gorm:"size:64;uniqueIndex;not null" json:"event_id"`
	EventType   string     `gorm:"size:64;not null" json:"event_type"`
	Payload     JSONMap    `gorm:"type:jsonb;not null" json:"payload"`
	Status      string     `gorm:"size:16;default:pending;index" json:"status"`
	Attempts    int        `gorm:"default:0" json:"attempts"`
	LastError   string     `gorm:"type:text" json:"last_error,omitempty"`
	NextRetryAt time.Time  `json:"next_retry_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func (OutboxEvent) TableName() string { return "webhook_outbox" }
