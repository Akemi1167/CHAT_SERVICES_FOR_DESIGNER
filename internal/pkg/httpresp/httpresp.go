// Package httpresp chuan hoa response JSON cua REST layer.
package httpresp

import (
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

type Envelope struct {
	Data  any        `json:"data,omitempty"`
	Error *ErrorBody `json:"error,omitempty"`
	Meta  any        `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func OK(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusOK).JSON(Envelope{Data: data})
}

func OKMeta(c *fiber.Ctx, data, meta any) error {
	return c.Status(fiber.StatusOK).JSON(Envelope{Data: data, Meta: meta})
}

func Created(c *fiber.Ctx, data any) error {
	return c.Status(fiber.StatusCreated).JSON(Envelope{Data: data})
}

func Fail(c *fiber.Ctx, err error) error {
	e := toAppErr(err)
	status := apperr.HTTPStatus(e)
	if status >= 500 {
		slog.Error("http request failed",
			"path", c.Path(), "method", c.Method(), "err", e.Error())
	}
	return c.Status(status).JSON(Envelope{Error: &ErrorBody{
		Code: e.Code, Message: e.Message, Details: e.Details,
	}})
}

// toAppErr: loi cua Fiber (route khong ton tai, sai method, body qua lon, can
// WebSocket upgrade...) phai ra dung status, khong duoc gom thanh 500.
func toAppErr(err error) *apperr.Error {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		return fromFiber(fe)
	}
	return apperr.From(err)
}

func fromFiber(fe *fiber.Error) *apperr.Error {
	switch fe.Code {
	case fiber.StatusNotFound:
		return apperr.NotFound("route_not_found",
			"Endpoint khong ton tai (kiem tra lai duong dan va tham so URL)")
	case fiber.StatusMethodNotAllowed:
		return apperr.Invalid("method_not_allowed", "HTTP method khong duoc ho tro cho endpoint nay")
	case fiber.StatusRequestEntityTooLarge:
		return apperr.Invalid("payload_too_large", "Noi dung gui len vuot qua HTTP_BODY_LIMIT_MB")
	case fiber.StatusUpgradeRequired:
		return apperr.Invalid("upgrade_required", "Endpoint nay chi nhan ket noi WebSocket")
	case fiber.StatusRequestTimeout:
		return apperr.Invalid("request_timeout", "Request qua thoi gian cho")
	default:
		if fe.Code >= 500 {
			return apperr.Internal("internal_error", "Co loi xay ra, vui long thu lai").Wrap(fe)
		}
		return apperr.Invalid("bad_request", fe.Message)
	}
}
