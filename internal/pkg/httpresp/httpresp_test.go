package httpresp

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

// Loi cua Fiber phai ra dung status + code, khong duoc gom het thanh 500.
func TestFailMapsFiberErrors(t *testing.T) {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error { return Fail(c, err) },
	})
	app.Get("/ok", func(c *fiber.Ctx) error { return OK(c, fiber.Map{"ok": true}) })
	app.Get("/app-error", func(c *fiber.Ctx) error {
		return Fail(c, apperr.NotFound("task_not_found", "Khong tim thay task"))
	})
	app.Get("/boom", func(c *fiber.Ctx) error { return fiber.ErrUpgradeRequired })

	cases := []struct {
		path       string
		wantStatus int
		wantCode   string
	}{
		{"/ok", 200, ""},
		{"/app-error", 404, "task_not_found"},
		{"/khong-ton-tai", 404, "route_not_found"}, // truoc day tra 500
		{"/boom", 400, "upgrade_required"},
	}
	for _, tc := range cases {
		resp, err := app.Test(httptest.NewRequest("GET", tc.path, nil))
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		if resp.StatusCode != tc.wantStatus {
			t.Errorf("%s: status = %d, want %d", tc.path, resp.StatusCode, tc.wantStatus)
		}
		body, _ := io.ReadAll(resp.Body)
		var env struct {
			Error *ErrorBody `json:"error"`
		}
		_ = json.Unmarshal(body, &env)
		if tc.wantCode == "" {
			if env.Error != nil {
				t.Errorf("%s: khong duoc co error, got %+v", tc.path, env.Error)
			}
			continue
		}
		if env.Error == nil || env.Error.Code != tc.wantCode {
			t.Errorf("%s: code = %v, want %q", tc.path, env.Error, tc.wantCode)
		}
	}
}
