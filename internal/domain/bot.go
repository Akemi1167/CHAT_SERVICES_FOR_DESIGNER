package domain

import (
	"time"

	"github.com/google/uuid"
)

// Bot duoc goi trong chat bang "@<slug>", vi du "@bot".
type Bot struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Slug        string    `gorm:"size:64;uniqueIndex;not null" json:"slug"`
	Name        string    `gorm:"size:128;not null" json:"name"`
	Description string    `gorm:"size:512" json:"description"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Bot) TableName() string { return "bots" }

// BotTemplate: dinh nghia "mau" (form) ma bot nhan.
// SchemaJSON de FE render form va BE validate so bo.
type BotTemplate struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	BotID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:ux_bot_template_code" json:"bot_id"`
	Code        string    `gorm:"size:64;not null;uniqueIndex:ux_bot_template_code" json:"code"`
	Name        string    `gorm:"size:255;not null" json:"name"`
	Description string    `gorm:"size:1024" json:"description"`
	Schema      JSONMap   `gorm:"type:jsonb" json:"schema,omitempty"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	Bot *Bot `gorm:"foreignKey:BotID" json:"bot,omitempty"`
}

func (BotTemplate) TableName() string { return "bot_templates" }

// BotSubmission: 1 lan user "gui mau" cho bot. Phase 1 chi can LUU.
// Phase sau: worker doc submission -> tao task/phieu sua... (status chuyen trang thai).
type BotSubmission struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	BotID          uuid.UUID  `gorm:"type:uuid;not null;index" json:"bot_id"`
	BotSlug        string     `gorm:"size:64;not null;index" json:"bot_slug"`
	TemplateID     *uuid.UUID `gorm:"type:uuid;index" json:"template_id,omitempty"`
	TemplateCode   string     `gorm:"size:64;index" json:"template_code"`
	ConversationID uuid.UUID  `gorm:"type:uuid;not null;index" json:"conversation_id"`
	MessageID      *uuid.UUID `gorm:"type:uuid;index" json:"message_id,omitempty"`
	ThreadID       *uuid.UUID `gorm:"type:uuid;index" json:"thread_id,omitempty"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;index" json:"user_id"`

	RawText string           `gorm:"type:text" json:"raw_text"`
	Payload JSONMap          `gorm:"type:jsonb" json:"payload,omitempty"`
	Status  SubmissionStatus `gorm:"size:32;not null;default:stored;index" json:"status"`
	Error   string           `gorm:"type:text" json:"error,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (BotSubmission) TableName() string { return "bot_submissions" }
