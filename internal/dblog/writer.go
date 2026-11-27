// Package dblog ghi log ung dung vao bang app_logs de debug: bat dong bo, theo lo,
// khong bao gio lam cham request (hang doi day thi bo dong moi, dem so dong bo).
//
// Nhan log qua logger.Sink (logger.AttachSink). Ghi bang session GORM tat log SQL,
// nen chinh cau INSERT khong sinh them log (khong vong lap). Loi cua chinh dblog
// in thang ra stderr — KHONG qua slog, vi slog lai quay ve day.
package dblog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/logger"
)

const (
	queueSize     = 4096
	batchSize     = 200
	flushEvery    = time.Second
	cleanupEvery  = time.Hour
	maxMsgRunes   = 4000
	maxAttrsBytes = 16 << 10
)

// Khoa duoc tach thanh cot rieng (de loc nhanh); con lai nam trong attrs.
// service / env giong nhau o moi dong nen bo.
var dropKeys = map[string]bool{"service": true, "env": true}

// Writer cai dat logger.Sink.
type Writer struct {
	db        *gorm.DB
	retention time.Duration
	queue     chan domain.AppLog
	dropped   atomic.Int64
}

// NewWriter: retention <= 0 = khong tu xoa.
func NewWriter(db *gorm.DB, retention time.Duration) *Writer {
	return &Writer{
		db:        db.Session(&gorm.Session{Logger: gormlogger.Discard}),
		retention: retention,
		queue:     make(chan domain.AppLog, queueSize),
	}
}

// Write: goi tu handler slog tren goroutine dang log — KHONG chan.
func (w *Writer) Write(e logger.Entry) {
	row, ok := toRow(e)
	if !ok {
		return
	}
	select {
	case w.queue <- row:
	default:
		w.dropped.Add(1)
	}
}

// Run ghi theo lo cho toi khi ctx dung, roi ghi not phan con lai.
func (w *Writer) Run(ctx context.Context) error {
	flush := time.NewTicker(flushEvery)
	defer flush.Stop()
	cleanup := time.NewTicker(cleanupEvery)
	defer cleanup.Stop()
	w.cleanup()

	batch := make([]domain.AppLog, 0, batchSize)
	write := func() {
		if len(batch) == 0 {
			return
		}
		// Context rieng: luc shutdown ctx da huy nhung van phai ghi not.
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.db.WithContext(c).CreateInBatches(batch, batchSize).Error; err != nil {
			fmt.Fprintf(os.Stderr, "dblog: ghi %d dong log vao DB that bai: %v\n", len(batch), err)
		}
		batch = batch[:0]
		if n := w.dropped.Swap(0); n > 0 {
			fmt.Fprintf(os.Stderr, "dblog: hang doi day, da bo %d dong log\n", n)
		}
	}

	for {
		select {
		case row := <-w.queue:
			batch = append(batch, row)
			if len(batch) >= batchSize {
				write()
			}
		case <-flush.C:
			write()
		case <-cleanup.C:
			w.cleanup()
		case <-ctx.Done():
			for {
				select {
				case row := <-w.queue:
					batch = append(batch, row)
					if len(batch) >= batchSize {
						write()
					}
				default:
					write()
					return nil
				}
			}
		}
	}
}

func (w *Writer) cleanup() {
	if w.retention <= 0 {
		return
	}
	c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cutoff := time.Now().Add(-w.retention)
	if err := w.db.WithContext(c).Where("created_at < ?", cutoff).Delete(&domain.AppLog{}).Error; err != nil {
		fmt.Fprintf(os.Stderr, "dblog: xoa log cu that bai: %v\n", err)
	}
}

// toRow tach cac khoa hay loc thanh cot; false = bo qua dong nay.
func toRow(e logger.Entry) (domain.AppLog, bool) {
	attrs := make(domain.JSONMap, len(e.Attrs))
	for k, v := range e.Attrs {
		if !dropKeys[k] {
			attrs[k] = v
		}
	}
	row := domain.AppLog{
		CreatedAt: e.Time,
		Level:     e.Level.String(),
		Msg:       truncate(e.Msg, maxMsgRunes),
	}
	if row.CreatedAt.IsZero() {
		row.CreatedAt = time.Now()
	}
	row.Component = takeString(attrs, "component", 64)
	row.RequestID = takeString(attrs, "request_id", 64)
	row.Method = takeString(attrs, "method", 8)
	row.Path = takeString(attrs, "path", 512)
	row.Status = takeInt(attrs, "status")
	row.TookMs = takeInt(attrs, "took_ms")

	// Healthcheck cua docker / load balancer goi vai giay 1 lan — chi la rac.
	if row.Msg == "http request" && row.Status < 400 && isProbe(row.Path) {
		return row, false
	}
	if len(attrs) > 0 {
		row.Attrs = capAttrs(attrs)
	}
	return row, true
}

func isProbe(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/metrics"
}

func takeString(m domain.JSONMap, key string, max int) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	delete(m, key)
	s, _ := v.(string)
	if s == "" {
		s = fmt.Sprint(v)
	}
	return truncate(s, max)
}

func takeInt(m domain.JSONMap, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		delete(m, key)
		return n
	case int64:
		delete(m, key)
		return int(n)
	case uint64:
		delete(m, key)
		return int(n)
	case float64:
		delete(m, key)
		return int(n)
	}
	return 0 // kieu la: de nguyen trong attrs
}

// capAttrs: attrs qua lon (vd log ca body) thi chi giu ban rut gon.
func capAttrs(m domain.JSONMap) domain.JSONMap {
	b, err := json.Marshal(m)
	if err != nil {
		return domain.JSONMap{"_error": "khong marshal duoc attrs: " + err.Error()}
	}
	if len(b) <= maxAttrsBytes {
		return m
	}
	return domain.JSONMap{"_truncated": true, "_raw": string(b[:maxAttrsBytes])}
}

func truncate(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// LevelName chuan hoa ten level tu query (debug|info|warn|error) sang dang luu.
func LevelName(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug.String()
	case "info":
		return slog.LevelInfo.String()
	case "warn", "warning":
		return slog.LevelWarn.String()
	case "error":
		return slog.LevelError.String()
	}
	return ""
}
