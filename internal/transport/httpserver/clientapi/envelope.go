// Package clientapi cai dat API cho web chat theo hop dong voi CMS:
//
//	Base:     /api/client/v1/chat
//	Auth:     Authorization: Bearer <access token CMS> (kiem bang JWKS)
//	Envelope: {statusCode, code, message, element}
//
// Day la lop bao ngoai cung tang service voi /api/v1 (dang dung cho Postman/CLI).
package clientapi

import (
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

type envelope struct {
	StatusCode int    `json:"statusCode"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Element    any    `json:"element,omitempty"`
}

func ok(c *fiber.Ctx, element any) error {
	return c.Status(fiber.StatusOK).JSON(envelope{
		StatusCode: fiber.StatusOK, Code: "SUCCESS", Message: "Success", Element: element,
	})
}

func fail(c *fiber.Ctx, err error) error {
	e := apperr.From(err)
	status := apperr.HTTPStatus(e)
	if status >= 500 {
		slog.Error("client api loi", "path", c.Path(), "err", e.Error())
	}
	return c.Status(status).JSON(envelope{
		StatusCode: status,
		Code:       upperCode(e.Code),
		Message:    e.Message,
		Element:    e.Details,
	})
}

// upperCode doi code noi bo (snake_case) sang dang CMS dung (UPPER_SNAKE).
func upperCode(code string) string {
	out := make([]byte, 0, len(code))
	for i := 0; i < len(code); i++ {
		ch := code[i]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		out = append(out, ch)
	}
	if len(out) == 0 {
		return "ERROR"
	}
	return string(out)
}
