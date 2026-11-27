package domain

import "time"

// AppLog: 1 dong log ung dung ghi vao DB de debug (xem internal/dblog).
type AppLog struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time `gorm:"not null;index:ix_app_logs_created,sort:desc" json:"created_at"`
	Level     string    `gorm:"size:8;not null" json:"level"`
	Msg       string    `gorm:"type:text;not null" json:"msg"`
	Component string    `gorm:"size:64;not null;default:''" json:"component,omitempty"`
	RequestID string    `gorm:"size:64;not null;default:''" json:"request_id,omitempty"`
	Method    string    `gorm:"size:8;not null;default:''" json:"method,omitempty"`
	Path      string    `gorm:"size:512;not null;default:''" json:"path,omitempty"`
	Status    int       `gorm:"not null;default:0" json:"status,omitempty"`
	TookMs    int       `gorm:"not null;default:0" json:"took_ms,omitempty"`
	Attrs     JSONMap   `gorm:"type:jsonb" json:"attrs,omitempty"`
}

func (AppLog) TableName() string { return "app_logs" }
