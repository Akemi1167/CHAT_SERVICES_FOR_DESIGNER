package httpserver

import (
	"crypto/subtle"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/dblog"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
)

// HeaderDebugKey: khoa rieng cho API xem log (DEBUG_API_KEY), khong dung chung JWT.
const HeaderDebugKey = "X-Debug-Key"

func debugKeyAuth(key string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if subtle.ConstantTimeCompare([]byte(c.Get(HeaderDebugKey)), []byte(key)) != 1 {
			return httpresp.Fail(c, apperr.Unauthorized("invalid_debug_key", HeaderDebugKey+" khong hop le"))
		}
		return c.Next()
	}
}

// debugLogs: GET /debug/logs — doc bang app_logs, moi nhat truoc.
//
//	level=warn            muc thap nhat (debug|info|warn|error)
//	component=ws          dung component
//	request_id=<id>       moi dong log cua 1 request (lay tu header X-Request-ID)
//	path=/bot/v1          path chua chuoi nay
//	q=mention             tim trong msg + attrs
//	status_min=400        chi request loi
//	since=, until=        RFC3339 (vd 2026-11-27T10:00:00+07:00) hoac khoang lui: 15m, 2h
//	limit=100 (<=500), before=<id> (trang sau = meta.next_before)
func debugLogs(r *dblog.Reader) fiber.Handler {
	return func(c *fiber.Ctx) error {
		f := dblog.Filter{
			MinLevel:  c.Query("level"),
			Component: c.Query("component"),
			RequestID: c.Query("request_id"),
			Path:      c.Query("path"),
			Q:         c.Query("q"),
			StatusMin: c.QueryInt("status_min", 0),
			Limit:     c.QueryInt("limit", 100),
			BeforeID:  int64(c.QueryInt("before", 0)),
		}
		var err error
		if f.Since, err = parseWhen(c.Query("since")); err != nil {
			return httpresp.Fail(c, err)
		}
		if f.Until, err = parseWhen(c.Query("until")); err != nil {
			return httpresp.Fail(c, err)
		}
		rows, next, err := r.List(c.UserContext(), f)
		if err != nil {
			return httpresp.Fail(c, err)
		}
		return httpresp.OKMeta(c, rows, fiber.Map{"count": len(rows), "next_before": next})
	}
}

// parseWhen nhan RFC3339 hoac khoang lui tinh tu bay gio ("15m", "2h", "7d").
func parseWhen(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t, nil
	}
	if strings.HasSuffix(s, "d") {
		if d, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h"); err == nil {
			t := time.Now().Add(-24 * d)
			return &t, nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil {
		t := time.Now().Add(-d)
		return &t, nil
	}
	return nil, apperr.Invalid("invalid_time", "since/until phai la RFC3339 hoac khoang lui nhu 15m, 2h, 7d: "+s)
}
