// Package botapi cai dat Bot API ma CMS goi sang Chat (luong 3 trong tai lieu tich hop).
//
//	Base: /bot/v1
//	Auth: header X-Chat-Service-Key (so sanh constant-time, khong co token)
//	Ghi:  moi request ghi co Idempotency-Key -> gap lai key thi tra ket qua cu
//	Loi:  {"code","message","retryable"} (khong boc envelope)
package botapi

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

const (
	HeaderServiceKey = "X-Chat-Service-Key"
	HeaderIdempotent = "Idempotency-Key"
)

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// ServiceKeyAuth chan moi request khong co dung khoa dung chung.
func ServiceKeyAuth(serviceKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if serviceKey == "" {
			return c.Status(fiber.StatusServiceUnavailable).JSON(errorBody{
				Code:      "service_key_not_configured",
				Message:   "Chat chua cau hinh CHAT_SERVICE_KEY",
				Retryable: false,
			})
		}
		got := c.Get(HeaderServiceKey)
		if subtle.ConstantTimeCompare([]byte(got), []byte(serviceKey)) != 1 {
			return c.Status(fiber.StatusUnauthorized).JSON(errorBody{
				Code:      "invalid_service_key",
				Message:   "X-Chat-Service-Key khong hop le",
				Retryable: false,
			})
		}
		return c.Next()
	}
}

// fail doi apperr sang dang loi cua Bot API.
func fail(c *fiber.Ctx, err error) error {
	e := apperr.From(err)
	status := apperr.HTTPStatus(e)
	code := e.Code
	switch e.Kind {
	case apperr.KindNotFound:
		code = "not_found"
	case apperr.KindInvalid:
		if code == "" {
			code = "invalid_request"
		}
	}
	if status >= 500 {
		slog.Error("bot api loi", "path", c.Path(), "err", e.Error())
	}
	return c.Status(status).JSON(errorBody{
		Code:      code,
		Message:   e.Message,
		Retryable: status >= 500, // loi he thong thi CMS duoc phep thu lai
	})
}

// toJSONMap doi response thanh JSONMap de luu ban ghi idempotency.
func toJSONMap(v any) domain.JSONMap {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m domain.JSONMap
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}
