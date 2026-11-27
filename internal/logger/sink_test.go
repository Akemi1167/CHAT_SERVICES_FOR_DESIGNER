package logger

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
)

type memSink struct {
	mu   sync.Mutex
	rows []Entry
}

func (m *memSink) Write(e Entry) { m.mu.Lock(); m.rows = append(m.rows, e); m.mu.Unlock() }

func TestTeeSink(t *testing.T) {
	sink := &memSink{}
	holder := &sinkHolder{}
	base := slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelInfo})
	l := slog.New(newTee(base, holder)).With("service", "chat")

	l.Info("truoc khi gan sink") // khong vao sink
	holder.cur.Store(&sinkState{sink: sink, min: slog.LevelDebug})

	ctx := WithRequestID(context.Background(), "req-1")
	l.With("component", "ws").WithGroup("http").InfoContext(ctx, "xong", "status", 200, "err", errors.New("boom"))
	l.Debug("debug van vao sink du stdout chi info", "k", 1)

	if len(sink.rows) != 2 {
		t.Fatalf("rows = %d", len(sink.rows))
	}
	got := sink.rows[0].Attrs
	want := map[string]any{"service": "chat", "component": "ws", "http.status": int64(200), "http.err": "boom", "request_id": "req-1"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %#v, want %#v (all=%v)", k, got[k], v, got)
		}
	}
	if sink.rows[1].Level != slog.LevelDebug || sink.rows[1].Msg != "debug van vao sink du stdout chi info" {
		t.Errorf("row 2 = %+v", sink.rows[1])
	}

	holder.cur.Store(&sinkState{sink: sink, min: slog.LevelWarn})
	l.Info("duoi nguong sink")
	if len(sink.rows) != 2 {
		t.Fatalf("info khong duoc vao sink khi min=warn")
	}
}

// Fiber tai su dung buffer: chuoi trong Entry phai la ban sao, khong bi doi theo.
func TestTeeSinkSaoChepChuoi(t *testing.T) {
	sink := &memSink{}
	holder := &sinkHolder{}
	holder.cur.Store(&sinkState{sink: sink, min: slog.LevelInfo})
	l := slog.New(newTee(slog.NewJSONHandler(io.Discard, nil), holder))

	buf := []byte("/api/v1/tasks")
	path := unsafeString(buf)
	l.Info("http request", "path", path)
	copy(buf, "/api/client/v1") // buffer bi dung lai cho request sau

	if got := sink.rows[0].Attrs["path"]; got != "/api/v1/tasks" {
		t.Fatalf("path bi doi theo buffer: %q", got)
	}
}
