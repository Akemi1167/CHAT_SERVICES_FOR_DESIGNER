package grpcserver

import (
	"context"

	"github.com/google/uuid"

	chatv1 "github.com/abc-workspace/chat-service/gen/go/chat/v1"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/bot"
	"github.com/abc-workspace/chat-service/internal/service/message"
)

type BotHandler struct {
	chatv1.UnimplementedBotServiceServer
	bots *bot.Service
	msgs *message.Service
}

func NewBotHandler(bots *bot.Service, msgs *message.Service) *BotHandler {
	return &BotHandler{bots: bots, msgs: msgs}
}

func (h *BotHandler) ListBots(ctx context.Context, _ *chatv1.ListBotsRequest) (*chatv1.ListBotsResponse, error) {
	items, err := h.bots.ListBots(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.Bot, 0, len(items))
	for _, b := range items {
		out = append(out, botTo(b))
	}
	return &chatv1.ListBotsResponse{Bots: out}, nil
}

func (h *BotHandler) ListBotTemplates(ctx context.Context, req *chatv1.ListBotTemplatesRequest) (*chatv1.ListBotTemplatesResponse, error) {
	b, tpls, err := h.bots.ListTemplates(ctx, req.GetBotSlug())
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.BotTemplate, 0, len(tpls))
	for _, t := range tpls {
		out = append(out, botTemplateTo(b.Slug, t))
	}
	return &chatv1.ListBotTemplatesResponse{Templates: out}, nil
}

// SubmitBotTemplate = "gui mau" tu form FE.
// Di qua message service de: tao tin nhan bot_command -> luu submission -> tra loi bot.
func (h *BotHandler) SubmitBotTemplate(ctx context.Context, req *chatv1.SubmitBotTemplateRequest) (*chatv1.SubmitBotTemplateResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	threadID, err := parseUUIDPtr(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	payload, err := domain.ParseJSONMap(req.GetPayloadJson())
	if err != nil {
		return nil, err
	}

	content := req.GetRawText()
	if content == "" {
		content = "@" + req.GetBotSlug() + " " + req.GetTemplateCode()
	}

	res, err := h.msgs.Send(ctx, message.SendInput{
		ActorID:         actor,
		ConversationID:  convID,
		Type:            domain.MessageBotCommand,
		Content:         content,
		ThreadID:        threadID,
		ClientMsgID:     req.GetClientMsgId(),
		BotSlug:         req.GetBotSlug(),
		TemplateCode:    req.GetTemplateCode(),
		TemplatePayload: payload,
		Metadata: domain.JSONMap{
			"bot_slug":      req.GetBotSlug(),
			"template_code": req.GetTemplateCode(),
			"payload":       map[string]any(payload),
		},
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.SubmitBotTemplateResponse{
		Submission: submissionTo(res.Submission),
		Message:    messageTo(res.Message),
		BotReply:   messageTo(res.BotReply),
	}, nil
}

func (h *BotHandler) ListBotSubmissions(ctx context.Context, req *chatv1.ListBotSubmissionsRequest) (*chatv1.ListBotSubmissionsResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	var convID *uuid.UUID
	if req.GetConversationId() != "" {
		id, err := parseUUID(req.GetConversationId(), "conversation_id")
		if err != nil {
			return nil, err
		}
		convID = &id
	}
	items, info, err := h.bots.ListSubmissions(ctx, bot.ListSubmissionsInput{
		BotSlug: req.GetBotSlug(), ConversationID: convID,
	}, pageFrom(req.GetPage()))
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.BotSubmission, 0, len(items))
	for i := range items {
		out = append(out, submissionTo(&items[i]))
	}
	return &chatv1.ListBotSubmissionsResponse{Submissions: out, PageInfo: pageInfoTo(info)}, nil
}
