// Package apperr dinh nghia error dung chung cho moi layer.
// Service layer chi tra apperr; transport (HTTP/gRPC/WS) tu map sang status code.
package apperr

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/grpc/codes"
)

type Kind string

const (
	KindInvalid      Kind = "invalid_argument"
	KindUnauthorized Kind = "unauthenticated"
	KindForbidden    Kind = "permission_denied"
	KindNotFound     Kind = "not_found"
	KindConflict     Kind = "conflict"
	KindInternal     Kind = "internal"
)

type Error struct {
	Kind    Kind           `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
	err     error
}

func (e *Error) Error() string {
	if e.err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.err }

func (e *Error) WithDetail(k string, v any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	e.Details[k] = v
	return e
}

func (e *Error) Wrap(err error) *Error {
	e.err = err
	return e
}

func newErr(kind Kind, code, msg string) *Error {
	return &Error{Kind: kind, Code: code, Message: msg}
}

func Invalid(code, msg string) *Error      { return newErr(KindInvalid, code, msg) }
func Unauthorized(code, msg string) *Error { return newErr(KindUnauthorized, code, msg) }
func Forbidden(code, msg string) *Error    { return newErr(KindForbidden, code, msg) }
func NotFound(code, msg string) *Error     { return newErr(KindNotFound, code, msg) }
func Conflict(code, msg string) *Error     { return newErr(KindConflict, code, msg) }
func Internal(code, msg string) *Error     { return newErr(KindInternal, code, msg) }

// From chuyen error bat ky thanh *Error (default: internal).
func From(err error) *Error {
	if err == nil {
		return nil
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return Internal("internal_error", "Co loi xay ra, vui long thu lai").Wrap(err)
}

func HTTPStatus(err error) int {
	switch From(err).Kind {
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func GRPCCode(err error) codes.Code {
	switch From(err).Kind {
	case KindInvalid:
		return codes.InvalidArgument
	case KindUnauthorized:
		return codes.Unauthenticated
	case KindForbidden:
		return codes.PermissionDenied
	case KindNotFound:
		return codes.NotFound
	case KindConflict:
		return codes.AlreadyExists
	default:
		return codes.Internal
	}
}

// Cac loi dung lai nhieu noi.
var (
	ErrUnauthenticated = Unauthorized("unauthenticated", "Token khong hop le hoac da het han")
	ErrNotMember       = Forbidden("not_conversation_member", "Ban khong thuoc cuoc hoi thoai nay")
	ErrConvNotFound    = NotFound("conversation_not_found", "Khong tim thay cuoc hoi thoai")
	// ErrBotDMReadOnly: chat rieng voi bot chi nhan tin cua bot (CMS chua nhan lenh trong chat rieng).
	ErrBotDMReadOnly      = Forbidden("bot_dm_read_only", "Chat riêng với bot chỉ nhận thông báo, không gửi tin được")
	ErrMessageNotFound    = NotFound("message_not_found", "Khong tim thay tin nhan")
	ErrCardNotFound       = NotFound("card_not_found", "Khong tim thay card voi ma nay")
	ErrBotNotFound        = NotFound("bot_not_found", "Bot khong ton tai hoac dang tat")
	ErrTemplateNotFound   = NotFound("bot_template_not_found", "Mau khong ton tai")
	ErrTaskNotFound       = NotFound("task_not_found", "Khong tim thay task")
	ErrAssignmentNotFound = NotFound("assignment_not_found", "Khong tim thay phan cong")
	ErrRevisionNotFound   = NotFound("revision_not_found", "Khong tim thay phieu sua")
	ErrTeamNotFound       = NotFound("team_not_found", "Khong tim thay to")
)
