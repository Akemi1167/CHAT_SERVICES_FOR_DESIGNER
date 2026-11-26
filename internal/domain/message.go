package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Message struct {
	ID             uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	ConversationID uuid.UUID   `gorm:"type:uuid;not null;index:ix_msg_conv_created,priority:1" json:"conversation_id"`
	SenderID       *uuid.UUID  `gorm:"type:uuid;index" json:"sender_id,omitempty"`
	SenderType     SenderType  `gorm:"size:16;not null;default:user" json:"sender_type"`
	Type           MessageType `gorm:"size:32;not null;default:text" json:"type"`
	Content        string      `gorm:"type:text" json:"content"`

	// ThreadID = ID cua message goc (card yeu cau/task). NULL = tin nhan trong nhom.
	ThreadID  *uuid.UUID `gorm:"type:uuid;index" json:"thread_id,omitempty"`
	ReplyToID *uuid.UUID `gorm:"type:uuid" json:"reply_to_id,omitempty"`

	Metadata JSONMap `gorm:"type:jsonb" json:"metadata,omitempty"`

	// Mentions = khoa nhac ten (MentionKey: actorId, hoac UUID khi user chua co actorId)
	// cua nguoi duoc nhac @. Da loc: chi thanh vien nhom, bo nguoi gui, bo trung.
	// MentionAll = nguoi gui dung @all (Mentions da gom moi thanh vien).
	Mentions   JSONStrings `gorm:"type:jsonb" json:"mentions,omitempty"`
	MentionAll bool        `gorm:"default:false" json:"mention_all,omitempty"`

	// ---- card abc.card/v1 do CMS dung (Chat chi luu va loc nut theo visibleTo) ----
	Card        JSONMap `gorm:"type:jsonb" json:"card,omitempty"`
	CardVersion int     `gorm:"default:0" json:"card_version"`
	// BotSender = {"id":1,"name":"Demo Designer Bot"} khi tin do bot CMS gui.
	BotSender JSONMap `gorm:"type:jsonb" json:"bot_sender,omitempty"`

	// Ghim tin nhan trong nhom (CMS goi POST /bot/v1/messages/:id/pin).
	Pinned   bool       `gorm:"default:false;index" json:"pinned"`
	PinnedAt *time.Time `json:"pinned_at,omitempty"`
	PinnedBy *uuid.UUID `gorm:"type:uuid" json:"pinned_by,omitempty"`

	// ClientMsgID: idempotency key do FE sinh, chong gui trung khi retry WS.
	ClientMsgID string `gorm:"size:64;index" json:"client_msg_id,omitempty"` // unique (conversation_id, client_msg_id) qua migration

	CreatedAt time.Time      `gorm:"index:ix_msg_conv_created,priority:2,sort:desc" json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	Attachments []Attachment `gorm:"foreignKey:MessageID" json:"attachments,omitempty"`
	Sender      *User        `gorm:"foreignKey:SenderID" json:"sender,omitempty"`

	// Field tinh toan, khong luu DB:
	// ReplyCount  = so tin tra loi TRUC TIEP tin nay (reply_to_id = id)
	// ThreadCount = so tin trong thread do tin nay mo (thread_id = id)
	ReplyCount  int64 `gorm:"-" json:"reply_count"`
	ThreadCount int64 `gorm:"-" json:"thread_count"`
}

func (Message) TableName() string { return "messages" }

type Attachment struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	MessageID  uuid.UUID `gorm:"type:uuid;not null;index" json:"message_id"`
	Kind       string    `gorm:"size:32;not null" json:"kind"` // image | video | gif | file
	URL        string    `gorm:"size:2048;not null" json:"url"`
	Name       string    `gorm:"size:512" json:"name"`
	MimeType   string    `gorm:"size:128" json:"mime_type"`
	SizeBytes  int64     `json:"size_bytes"`
	Width      int32     `json:"width"`
	Height     int32     `json:"height"`
	DurationMs int32     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

func (Attachment) TableName() string { return "attachments" }

// MessageReceipt luu "ai da doc tin nhan nao" (danh sach nguoi da xem).
type MessageReceipt struct {
	MessageID uuid.UUID `gorm:"type:uuid;primaryKey" json:"message_id"`
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey" json:"user_id"`
	SeenAt    time.Time `json:"seen_at"`
}

func (MessageReceipt) TableName() string { return "message_receipts" }
