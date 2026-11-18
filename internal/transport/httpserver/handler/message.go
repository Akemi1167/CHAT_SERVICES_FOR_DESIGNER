package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

type MessageHandler struct {
	msgs *message.Service
}

func NewMessageHandler(msgs *message.Service) *MessageHandler {
	return &MessageHandler{msgs: msgs}
}

type sendMessageReq struct {
	Type        string              `json:"type"`
	Content     string              `json:"content"`
	ThreadID    string              `json:"thread_id"`
	ReplyToID   string              `json:"reply_to_id"`
	Metadata    domain.JSONMap      `json:"metadata"`
	Attachments []attachmentPayload `json:"attachments"`
	ClientMsgID string              `json:"client_msg_id"`
}

type attachmentPayload struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Name       string `json:"name"`
	MimeType   string `json:"mime_type"`
	SizeBytes  int64  `json:"size_bytes"`
	Width      int32  `json:"width"`
	Height     int32  `json:"height"`
	DurationMs int32  `json:"duration_ms"`
}

func (p attachmentPayload) toDomain() domain.Attachment {
	return domain.Attachment{
		ID: uuid.New(), Kind: p.Kind, URL: p.URL, Name: p.Name,
		MimeType: p.MimeType, SizeBytes: p.SizeBytes,
		Width: p.Width, Height: p.Height, DurationMs: p.DurationMs,
	}
}

func attachmentsToDomain(in []attachmentPayload) []domain.Attachment {
	out := make([]domain.Attachment, 0, len(in))
	for _, a := range in {
		if a.URL == "" {
			continue
		}
		out = append(out, a.toDomain())
	}
	return out
}

func (h *MessageHandler) Send(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	threadID, err := optionalUUID(req.ThreadID, "thread_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	replyTo, err := optionalUUID(req.ReplyToID, "reply_to_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	res, err := h.msgs.Send(c.UserContext(), message.SendInput{
		ActorID:        id.UserID,
		ConversationID: convID,
		Type:           domain.MessageType(req.Type),
		Content:        req.Content,
		ThreadID:       threadID,
		ReplyToID:      replyTo,
		Metadata:       req.Metadata,
		Attachments:    attachmentsToDomain(req.Attachments),
		ClientMsgID:    req.ClientMsgID,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.Created(c, fiber.Map{
		"message":    res.Message,
		"bot_reply":  res.BotReply,
		"submission": res.Submission,
		"duplicated": res.Duplicated,
	})
}

func (h *MessageHandler) List(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	threadID, err := optionalUUID(c.Query("thread_id"), "thread_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	items, info, err := h.msgs.List(c.UserContext(), message.ListInput{
		ActorID:        id.UserID,
		ConversationID: convID,
		ThreadID:       threadID,
		OnlyRoot:       c.QueryBool("only_root", false),
	}, pageFromQuery(c))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, items, info)
}

func (h *MessageHandler) Search(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID := uuid.Nil
	if v := c.Query("conversation_id"); v != "" {
		parsed, err := uuid.Parse(v)
		if err != nil {
			return httpresp.Fail(c, apperr.Invalid("invalid_uuid", "conversation_id phai la UUID"))
		}
		convID = parsed
	}
	senderID, err := optionalUUID(c.Query("sender_id"), "sender_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	from, err := optionalDate(c.Query("from_date"), "from_date")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	to, err := optionalDate(c.Query("to_date"), "to_date")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	items, info, err := h.msgs.Search(c.UserContext(), message.SearchInput{
		ActorID: id.UserID, ConversationID: convID, Query: c.Query("q"),
		SenderID: senderID, FromDate: from, ToDate: to,
	}, pageFromQuery(c))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, items, info)
}

type markReadReq struct {
	MessageID string `json:"message_id"`
}

func (h *MessageHandler) MarkRead(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req markReadReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	msgID, err := uuid.Parse(req.MessageID)
	if err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_uuid", "message_id phai la UUID"))
	}
	unread, err := h.msgs.MarkRead(c.UserContext(), id.UserID, convID, msgID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, fiber.Map{"unread_count": unread})
}

// Replies tra cac tin nhan tra loi 1 tin nhan.
//
//	mode=direct (mac dinh) — tin co reply_to_id = tin nay (nut Reply tren UI)
//	mode=thread            — toan bo thread do tin nay mo (thread_id = tin nay)
func (h *MessageHandler) Replies(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	msgID, err := paramUUID(c, "message_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	res, err := h.msgs.Replies(c.UserContext(), message.RepliesInput{
		ActorID:   id.UserID,
		MessageID: msgID,
		Mode:      repository.ReplyMode(c.Query("mode")),
	}, pageFromQuery(c))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, fiber.Map{
		"root":         res.Root,
		"replies":      res.Replies,
		"reply_count":  res.DirectCount,
		"thread_count": res.ThreadCount,
	}, res.PageInfo)
}

// Readers tra danh sach nguoi da xem 1 tin nhan.
func (h *MessageHandler) Readers(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	msgID, err := paramUUID(c, "message_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	receipts, err := h.msgs.Readers(c.UserContext(), id.UserID, msgID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, receipts)
}

func (h *MessageHandler) Delete(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	msgID, err := paramUUID(c, "message_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	if err := h.msgs.Delete(c.UserContext(), id.UserID, msgID); err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, fiber.Map{"deleted": true})
}

func optionalUUID(s, field string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, apperr.Invalid("invalid_uuid", field+" phai la UUID")
	}
	return &id, nil
}

func optionalDate(s, field string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, apperr.Invalid("invalid_date", field+" phai dang YYYY-MM-DD hoac RFC3339")
}
