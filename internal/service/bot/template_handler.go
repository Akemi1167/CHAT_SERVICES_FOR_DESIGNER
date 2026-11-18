package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// TemplateHandler: handler mac dinh — "nhan mau va luu xuong DB".
// Phase 1 chi luu (status = stored). Phase 2: worker doc bot_submissions
// de tao task/phieu sua... ma khong phai sua gi o tang chat.
type TemplateHandler struct {
	store repository.Store
}

func NewTemplateHandler(store repository.Store) *TemplateHandler {
	return &TemplateHandler{store: store}
}

var _ Handler = (*TemplateHandler)(nil)

func (h *TemplateHandler) Handle(ctx context.Context, req *Request) (*Result, error) {
	bots := h.store.Bots()

	// Khong chi dinh mau -> tra ve huong dan + danh sach mau.
	if req.TemplateCode == "" {
		templates, err := bots.ListTemplates(ctx, req.Bot.ID)
		if err != nil {
			return nil, err
		}
		return &Result{ReplyText: helpText(req.Bot, templates), ReplyMeta: domain.JSONMap{
			"kind":      "help",
			"templates": templateCodes(templates),
		}}, nil
	}

	tpl, err := bots.GetTemplate(ctx, req.Bot.ID, req.TemplateCode)
	if err != nil {
		if apperr.From(err).Kind == apperr.KindNotFound {
			templates, lerr := bots.ListTemplates(ctx, req.Bot.ID)
			if lerr != nil {
				return nil, lerr
			}
			return nil, apperr.Invalid("bot_template_not_found",
				fmt.Sprintf("Mau \"%s\" khong ton tai. Cac mau dang co: %s",
					req.TemplateCode, strings.Join(templateCodes(templates), ", "))).
				WithDetail("templates", templateCodes(templates))
		}
		return nil, err
	}

	if err := validatePayload(tpl, req.Payload); err != nil {
		return nil, err
	}

	sub := &domain.BotSubmission{
		ID:             uuid.New(),
		BotID:          req.Bot.ID,
		BotSlug:        req.Bot.Slug,
		TemplateID:     &tpl.ID,
		TemplateCode:   tpl.Code,
		ConversationID: req.ConversationID,
		MessageID:      req.MessageID,
		ThreadID:       req.ThreadID,
		UserID:         req.UserID,
		RawText:        req.RawText,
		Payload:        req.Payload,
		Status:         domain.SubmissionStored,
	}
	if err := bots.CreateSubmission(ctx, sub); err != nil {
		return nil, err
	}

	return &Result{
		Submission: sub,
		ReplyText:  fmt.Sprintf("Da luu mau \"%s\" (ma phieu: %s).", tpl.Name, shortID(sub.ID)),
		ReplyMeta: domain.JSONMap{
			"kind":          "submission_stored",
			"submission_id": sub.ID.String(),
			"template_code": tpl.Code,
			"payload":       map[string]any(req.Payload),
		},
	}, nil
}

// validatePayload kiem tra field required khai bao trong BotTemplate.Schema:
//
//	{"fields":[{"key":"so_luong","label":"So luong","type":"number","required":true}]}
func validatePayload(tpl *domain.BotTemplate, payload domain.JSONMap) error {
	raw, ok := tpl.Schema["fields"]
	if !ok {
		return nil
	}
	fields, ok := raw.([]any)
	if !ok {
		return nil
	}

	var missing []string
	for _, f := range fields {
		fm, ok := f.(map[string]any)
		if !ok {
			continue
		}
		required, _ := fm["required"].(bool)
		key, _ := fm["key"].(string)
		if !required || key == "" {
			continue
		}
		v, exists := payload[key]
		if !exists || v == nil || v == "" {
			label, _ := fm["label"].(string)
			if label == "" {
				label = key
			}
			missing = append(missing, fmt.Sprintf("%s (%s)", label, key))
		}
	}
	if len(missing) > 0 {
		return apperr.Invalid("bot_template_missing_fields",
			"Thieu truong bat buoc: "+strings.Join(missing, ", ")).
			WithDetail("missing", missing)
	}
	return nil
}

func helpText(b *domain.Bot, templates []domain.BotTemplate) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "@%s — %s\n", b.Slug, b.Description)
	if len(templates) == 0 {
		sb.WriteString("Chua co mau nao duoc cau hinh.")
		return sb.String()
	}
	sb.WriteString("Cac mau dang co:\n")
	for _, t := range templates {
		fmt.Fprintf(&sb, "- %s: %s\n", t.Code, t.Name)
	}
	fmt.Fprintf(&sb, "Cu phap: @%s <ma_mau> truong=gia_tri ...", b.Slug)
	return sb.String()
}

func templateCodes(templates []domain.BotTemplate) []string {
	out := make([]string, 0, len(templates))
	for _, t := range templates {
		out = append(out, t.Code)
	}
	return out
}

func shortID(id uuid.UUID) string {
	s := id.String()
	return s[:8]
}
