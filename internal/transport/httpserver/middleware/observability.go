package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/logger"
)

// RequestID gan X-Request-ID vao context + response header (de trace log).
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rid := c.Get("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set("X-Request-ID", rid)
		c.SetUserContext(logger.WithRequestID(c.UserContext(), rid))
		return c.Next()
	}
}

// AccessLog log 1 dong moi request.
func AccessLog() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		// Loi tra ve qua chain (vd route khong ton tai, handler tra error) chi duoc
		// doi thanh status that khi ErrorHandler chay — goi no o day de log dung status,
		// truoc day dong log ghi 200 cho request thuc te tra 404.
		if err := c.Next(); err != nil {
			if herr := c.App().ErrorHandler(c, err); herr != nil {
				_ = c.SendStatus(fiber.StatusInternalServerError)
			}
		}
		slog.Info("http request",
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"took_ms", time.Since(start).Milliseconds(),
			"request_id", c.GetRespHeader("X-Request-ID"),
			"ip", c.IP(),
		)
		return nil
	}
}
