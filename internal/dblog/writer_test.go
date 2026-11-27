package dblog

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/abc-workspace/chat-service/internal/logger"
)

func TestToRow(t *testing.T) {
	now := time.Now()
	row, ok := toRow(logger.Entry{Time: now, Level: slog.LevelWarn, Msg: "http request", Attrs: map[string]any{
		"service": "chat-service", "env": "local",
		"method": "POST", "path": "/bot/v1/messages", "status": int64(500), "took_ms": int64(12),
		"request_id": "req-1", "component": "botapi", "ip": "1.2.3.4",
	}})
	if !ok {
		t.Fatal("bi bo qua")
	}
	if row.Level != "WARN" || row.Method != "POST" || row.Path != "/bot/v1/messages" || row.Status != 500 ||
		row.TookMs != 12 || row.RequestID != "req-1" || row.Component != "botapi" {
		t.Fatalf("row = %+v", row)
	}
	if len(row.Attrs) != 1 || row.Attrs["ip"] != "1.2.3.4" {
		t.Fatalf("attrs con lai = %v (service/env/cot da tach phai bi bo)", row.Attrs)
	}
}

func TestToRowBoHealthcheck(t *testing.T) {
	for path, want := range map[string]bool{"/healthz": false, "/readyz": false, "/api/client/v1/chat/bootstrap": true} {
		_, ok := toRow(logger.Entry{Level: slog.LevelInfo, Msg: "http request", Attrs: map[string]any{"path": path, "status": int64(200)}})
		if ok != want {
			t.Errorf("%s: ok=%v want %v", path, ok, want)
		}
	}
	// healthcheck LOI thi van ghi
	if _, ok := toRow(logger.Entry{Level: slog.LevelInfo, Msg: "http request", Attrs: map[string]any{"path": "/readyz", "status": int64(503)}}); !ok {
		t.Error("readyz 503 phai duoc ghi")
	}
}

func TestToRowCatAttrsQuaLon(t *testing.T) {
	row, _ := toRow(logger.Entry{Level: slog.LevelError, Msg: "x", Attrs: map[string]any{"body": strings.Repeat("a", maxAttrsBytes+100)}})
	if row.Attrs["_truncated"] != true {
		t.Fatalf("attrs khong bi cat: %d khoa", len(row.Attrs))
	}
}

func TestLevelsFrom(t *testing.T) {
	if got := strings.Join(levelsFrom("warn"), ","); got != "WARN,ERROR" {
		t.Fatal(got)
	}
	if levelsFrom("") != nil {
		t.Fatal("rong = khong loc")
	}
}
