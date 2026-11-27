// Package database khoi tao ket noi Postgres qua GORM.
package database

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/domain"
)

// AllModels: danh sach model dung cho AutoMigrate (dev) va kiem tra schema.
func AllModels() []any {
	return []any{
		&domain.User{},
		&domain.Conversation{},
		&domain.ConversationMember{},
		&domain.Message{},
		&domain.Attachment{},
		&domain.MessageReceipt{},
		&domain.Bot{},
		&domain.BotTemplate{},
		&domain.BotSubmission{},
		&domain.Notification{},
		&domain.IdempotencyRecord{},
		&domain.OutboxEvent{},
		&domain.AppLog{},
	}
}

func Open(cfg config.Postgres) (*gorm.DB, error) {
	logLevel := gormlogger.Warn
	if cfg.LogSQL {
		logLevel = gormlogger.Info
	}

	var gl gormlogger.Interface
	if cfg.LogPretty {
		// Dev: in SQL nhieu dong, co mau, da dien gia tri tham so.
		gl = NewPrettyLogger(os.Stdout, logLevel, cfg.SlowQuery)
	} else {
		// Prod: 1 dong JSON qua slog, tham so giu dang $1 de khong lot du lieu.
		gl = gormlogger.New(
			newSlogWriter(),
			gormlogger.Config{
				SlowThreshold:             cfg.SlowQuery,
				LogLevel:                  logLevel,
				IgnoreRecordNotFoundError: true,
				ParameterizedQueries:      true,
			},
		)
	}

	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger:                 gl,
		SkipDefaultTransaction: true,
		NowFunc:                func() time.Time { return time.Now().UTC() },
		TranslateError:         true, // bien loi driver thanh gorm.ErrDuplicatedKey...
	})
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: sql handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	if cfg.AutoMigrate {
		slog.Warn("POSTGRES_AUTO_MIGRATE dang bat - chi dung cho local, prod phai dung golang-migrate")
		if err := db.AutoMigrate(AllModels()...); err != nil {
			return nil, fmt.Errorf("database: auto migrate: %w", err)
		}
	}
	return db, nil
}

func Close(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

type slogWriter struct{ l *slog.Logger }

func newSlogWriter() gormlogger.Writer {
	return &slogWriter{l: slog.Default()}
}

func (w *slogWriter) Printf(format string, args ...any) {
	w.l.Info("gorm", "sql", fmt.Sprintf(format, args...))
}
