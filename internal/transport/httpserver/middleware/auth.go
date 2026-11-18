// Package middleware chua cac middleware dung chung cho Fiber.
package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/service/auth"
)

const IdentityKey = "identity"

// Auth (REST): uu tien header Authorization, roi moi den ?token=.
func Auth(authSvc *auth.Service) fiber.Handler {
	return authWith(authSvc, tokenFromHeaderFirst)
}

// AuthWS (WebSocket): uu tien ?token= trong URL.
//
// Ly do: browser khong set duoc header khi mo WebSocket nen FE luon dung ?token=.
// Ngoai ra cac tool (vd request WebSocket luu trong Postman collection co Bearer auth
// ke thua) hay gan san header Authorization cua mot token khac — neu uu tien header thi
// ket noi se dang nhap bang NGUOI KHAC, van nhan duoc frame "ready" nhung khong bao gio
// nhan duoc tin nhan cua room, rat kho doan ra.
func AuthWS(authSvc *auth.Service) fiber.Handler {
	return authWith(authSvc, tokenFromQueryFirst)
}

func authWith(authSvc *auth.Service, pick func(*fiber.Ctx) string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := pick(c)
		if token == "" {
			return httpresp.Fail(c, apperr.ErrUnauthenticated)
		}

		identity, err := authSvc.Authenticate(c.UserContext(), token)
		if err != nil {
			return httpresp.Fail(c, err)
		}

		c.Locals(IdentityKey, identity)
		c.SetUserContext(auth.WithIdentity(c.UserContext(), identity))
		return c.Next()
	}
}

func tokenFromHeaderFirst(c *fiber.Ctx) string {
	return firstNonEmpty(
		c.Get(fiber.HeaderAuthorization),
		c.Query("token"),
		c.Get("Sec-WebSocket-Protocol"),
	)
}

func tokenFromQueryFirst(c *fiber.Ctx) string {
	return firstNonEmpty(
		c.Query("token"),
		c.Get("Sec-WebSocket-Protocol"),
		c.Get(fiber.HeaderAuthorization),
	)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// Identity doc identity da duoc Auth gan vao.
func Identity(c *fiber.Ctx) (*auth.Identity, error) {
	id, ok := c.Locals(IdentityKey).(*auth.Identity)
	if !ok || id == nil {
		return nil, apperr.ErrUnauthenticated
	}
	return id, nil
}
