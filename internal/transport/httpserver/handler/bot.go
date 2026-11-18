package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/service/bot"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

type BotHandler struct {
	bots *bot.Service
	msgs *message.Service
}

func NewBotHandler(bots *bot.Service, msgs *message.Service) *BotHandler {
	return &BotHandler{bots: bots, msgs: msgs}
}

func (h *BotHandler) ListBots(c *fiber.Ctx) error {
	items, err := h.bots.ListBots(c.UserContext())
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, items)
}

// ListTemplates: FE dung schema nay de render form "gui mau".
func (h *BotHandler) ListTemplates(c *fiber.Ctx) error {
	b, tpls, err := h.bots.ListTemplates(c.UserContext(), c.Params("slug"))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, fiber.Map{"bot": b, "templates": tpls})
}

type submitTemplateReq struct {
	ConversationID string         `json:"conversation_id"`
	ThreadID       string         `json:"thread_id"`
	TemplateCode   string         `json:"template_code"`
	Payload        domain.JSONMap `json:"payload"`
	RawText        string         `json:"raw_text"`
	ClientMsgID    string         `json:"client_msg_id"`
}

// Submit = "gui mau" tu form. Tao tin nhan bot_command -> bot luu submission -> tra loi.
func (h *BotHandler) Submit(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	slug := c.Params("slug")
	var req submitTemplateReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	convID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_uuid", "conversation_id phai la UUID"))
	}
	threadID, err := optionalUUID(req.ThreadID, "thread_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	content := req.RawText
	if content == "" {
		content = "@" + slug + " " + req.TemplateCode
	}

	res, err := h.msgs.Send(c.UserContext(), message.SendInput{
		ActorID:         id.UserID,
		ConversationID:  convID,
		Type:            domain.MessageBotCommand,
		Content:         content,
		ThreadID:        threadID,
		ClientMsgID:     req.ClientMsgID,
		BotSlug:         slug,
		TemplateCode:    req.TemplateCode,
		TemplatePayload: req.Payload,
		Metadata: domain.JSONMap{
			"bot_slug":      slug,
			"template_code": req.TemplateCode,
			"payload":       map[string]any(req.Payload),
		},
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.Created(c, fiber.Map{
		"message":    res.Message,
		"bot_reply":  res.BotReply,
		"submission": res.Submission,
	})
}

func (h *BotHandler) ListSubmissions(c *fiber.Ctx) error {
	if _, err := middleware.Identity(c); err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := optionalUUID(c.Query("conversation_id"), "conversation_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var status *domain.SubmissionStatus
	if v := c.Query("status"); v != "" {
		s := domain.SubmissionStatus(v)
		status = &s
	}
	items, info, err := h.bots.ListSubmissions(c.UserContext(), bot.ListSubmissionsInput{
		BotSlug: c.Params("slug"), ConversationID: convID, Status: status,
	}, pageFromQuery(c))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, items, info)
}
