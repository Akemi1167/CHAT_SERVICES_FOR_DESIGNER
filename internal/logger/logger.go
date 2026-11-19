// Package logger khoi tao slog global (structured log => de debug/grep).
package logger

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type ctxKey string

const requestIDKey ctxKey = "request_id"

// Options cau hinh log.
//
//	File="" , Stdout=true   => chi stdout (mac dinh, prod dung cai nay:
//	                           docker/k8s thu gom log tu stdout)
//	File=... , Stdout=true   => ghi CA hai noi
//	File=... , Stdout=false  => CHI ghi file; console docker sach, chi con
//	                           thu in thang ra stdout (vd SQL pretty log)
//
// Stdout=false ma File="" la vo nghia (mat sach log) => tu quay ve stdout.
type Options struct {
	Level   string // debug | info | warn | error
	Format  string // json | text
	File    string // vd ./logs/app.log — de trong la tat
	Stdout  bool   // false => khong in log ra stdout (can co File)
	Service string
	Env     string
}

// Setup gan slog mac dinh. Closer tra ve dung de dong file log khi app dung
// (noop neu khong ghi file) — luon goi, vi du: defer closer.Close().
func Setup(o Options) (*slog.Logger, io.Closer) {
	lv := parseLevel(o.Level)

	var (
		out     io.Writer = os.Stdout
		closer  io.Closer = noopCloser{}
		fileErr error
		quiet   bool // da tat stdout that su chua
	)
	if o.File != "" {
		f, err := openLogFile(o.File)
		if err != nil {
			// Chua co logger de bao loi => nho lai, log sau khi SetDefault.
			// Luon con stdout nen khong mat log.
			fileErr = err
		} else {
			closer = f
			if o.Stdout {
				out = io.MultiWriter(os.Stdout, f)
			} else {
				out = f
				quiet = true
			}
		}
	}

	opts := &slog.HandlerOptions{Level: lv, AddSource: lv == slog.LevelDebug}
	var h slog.Handler
	if strings.EqualFold(o.Format, "text") {
		h = slog.NewTextHandler(out, opts)
	} else {
		h = slog.NewJSONHandler(out, opts)
	}

	l := slog.New(h).With("service", o.Service, "env", o.Env)
	slog.SetDefault(l)

	switch {
	case fileErr != nil:
		// Khong mo duoc file khong phai ly do de service chet.
		l.Warn("khong mo duoc LOG_FILE, chi ghi stdout", "file", o.File, "err", fileErr)
	case quiet:
		// Dong duy nhat in ra stdout, de biet log di dau ma tim.
		fmt.Fprintf(os.Stdout, "log -> %s (LOG_STDOUT=false, console nay chi con SQL log neu bat POSTGRES_LOG_SQL)\n", o.File)
		l.Info("log chi ghi ra file", "file", o.File)
	case o.File != "":
		l.Info("log ghi ra ca stdout va file", "file", o.File)
	case !o.Stdout:
		l.Warn("LOG_STDOUT=false nhung khong co LOG_FILE => van ghi stdout, neu khong se mat sach log")
	}
	return l, closer
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// openLogFile mo file o che do append (khong xoa log cu khi restart) va tu tao
// thu muc cha neu chua co.
func openLogFile(path string) (*os.File, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// FromContext tra logger da gan request_id (neu co).
func FromContext(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if id := RequestID(ctx); id != "" {
		return l.With("request_id", id)
	}
	return l
}
