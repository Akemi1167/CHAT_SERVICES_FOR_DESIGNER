package httpserver

import (
	"crypto/subtle"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/docs"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
)

// HeaderDocsKey: khoa rieng cho API tai lieu, KHONG dung chung voi
// CHAT_SERVICE_KEY — lo khoa nay thi chi lo tai lieu, khong goi duoc Bot API.
const HeaderDocsKey = "X-Docs-Key"

// docsKeyAuth chan request khong co dung khoa (so sanh constant-time).
func docsKeyAuth(key string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if subtle.ConstantTimeCompare([]byte(c.Get(HeaderDocsKey)), []byte(key)) != 1 {
			return httpresp.Fail(c, apperr.Unauthorized("invalid_docs_key", HeaderDocsKey+" khong hop le"))
		}
		return c.Next()
	}
}

// flowDoc tra nguyen van docs/FLOW.md (markdown) da nhung vao binary luc build.
func flowDoc(c *fiber.Ctx) error {
	c.Set(fiber.HeaderContentType, "text/markdown; charset=utf-8")
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.Send(docs.Flow)
}
