package task

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/service/bot"
)

// Ma mau duoc bot hieu (seed trong migration 000002).
const (
	TemplateYeuCau   = "yeu_cau"
	TemplatePhieuSua = "phieu_sua"
)

// BotHandler boc TemplateHandler: van LUU mau xuong bot_submissions (audit),
// sau do bien mau thanh task / phieu sua thuc su.
//
// Them mau moi co hanh dong: them 1 case trong Handle + 1 migration INSERT bot_templates.
type BotHandler struct {
	inner bot.Handler // thuong la bot.TemplateHandler
	tasks *Service
}

func NewBotHandler(inner bot.Handler, tasks *Service) *BotHandler {
	return &BotHandler{inner: inner, tasks: tasks}
}

var _ bot.Handler = (*BotHandler)(nil)

func (h *BotHandler) Handle(ctx context.Context, req *bot.Request) (*bot.Result, error) {
	// 1) Luu mau (validate field bat buoc theo schema).
	res, err := h.inner.Handle(ctx, req)
	if err != nil || res == nil || res.Submission == nil {
		return res, err
	}

	// 2) Bien mau thanh nghiep vu.
	switch req.TemplateCode {
	case TemplateYeuCau:
		return h.createTask(ctx, req, res)
	case TemplatePhieuSua:
		return h.createRevision(ctx, req, res)
	default:
		return res, nil // mau chi can luu
	}
}

func (h *BotHandler) createTask(ctx context.Context, req *bot.Request, res *bot.Result) (*bot.Result, error) {
	p := req.Payload
	deadline, err := payloadTime(p, "deadline")
	if err != nil {
		return nil, h.failSubmission(ctx, res, err)
	}

	in := CreateInput{
		ActorID:        req.UserID,
		ConversationID: req.ConversationID,
		ThreadID:       req.ThreadID,
		Title:          payloadString(p, "tieu_de", "title"),
		Description:    payloadString(p, "ghi_chu", "mo_ta", "description"),
		TeamCode:       payloadString(p, "to", "team", "team_code"),
		Quantity:       payloadInt(p, "so_luong", "quantity"),
		Quota:          payloadString(p, "dinh_muc", "quota"),
		Deadline:       deadline,
		Priority:       domain.TaskPriority(payloadString(p, "uu_tien", "priority")),
		SubmissionID:   &res.Submission.ID,
		Metadata:       domain.JSONMap{"source": "bot", "bot_slug": req.Bot.Slug},
	}
	// Thread: neu tin nhan "@bot ..." nam trong thread nao thi task thuoc thread do,
	// nguoc lai card task se tro thanh thread goc moi.
	if in.ThreadID == nil && req.MessageID != nil {
		in.ThreadID = req.ThreadID
	}
	if id, ok := payloadUUID(p, "nguoi_phu_trach", "assignee_id"); ok {
		in.AssigneeIDs = []uuid.UUID{id}
	}

	t, err := h.tasks.Create(ctx, in)
	if err != nil {
		return nil, h.failSubmission(ctx, res, err)
	}

	_ = h.tasks.store.Bots().UpdateSubmissionStatus(ctx, res.Submission.ID, domain.SubmissionDone, "")
	res.Submission.Status = domain.SubmissionDone
	// Card task da duoc dang vao hoi thoai => bot khong can tra loi them.
	res.Silent = true
	res.ReplyText = fmt.Sprintf("Da tao yeu cau %s", t.Code)
	res.ReplyMeta = domain.JSONMap{
		"kind": "task_created", "task_id": t.ID.String(), "task_code": t.Code,
	}
	return res, nil
}

func (h *BotHandler) createRevision(ctx context.Context, req *bot.Request, res *bot.Result) (*bot.Result, error) {
	p := req.Payload
	code := strings.ToUpper(strings.TrimSpace(payloadString(p, "ma_task", "task_code")))
	if code == "" {
		return nil, h.failSubmission(ctx, res,
			apperr.Invalid("task_code_required", "Phieu sua phai co ma_task"))
	}

	t, err := h.tasks.GetByCode(ctx, req.UserID, code)
	if err != nil {
		return nil, h.failSubmission(ctx, res, err)
	}

	rev, _, err := h.tasks.CreateRevision(ctx, CreateRevisionInput{
		ActorID:      req.UserID,
		TaskID:       t.ID,
		Reason:       payloadString(p, "nguyen_nhan", "reason"),
		RevisionType: payloadString(p, "loai_sua", "revision_type"),
		Quantity:     payloadInt(p, "so_luong", "quantity"),
		SubmissionID: &res.Submission.ID,
		MessageID:    req.MessageID,
		Metadata:     domain.JSONMap{"source": "bot", "note": payloadString(p, "ghi_chu")},
	})
	if err != nil {
		return nil, h.failSubmission(ctx, res, err)
	}

	_ = h.tasks.store.Bots().UpdateSubmissionStatus(ctx, res.Submission.ID, domain.SubmissionDone, "")
	res.Submission.Status = domain.SubmissionDone
	res.Silent = true
	res.ReplyText = fmt.Sprintf("Da tao phieu sua %s cho %s", rev.Code, t.Code)
	res.ReplyMeta = domain.JSONMap{
		"kind": "revision_created", "revision_id": rev.ID.String(),
		"revision_code": rev.Code, "task_code": t.Code,
	}
	return res, nil
}

// failSubmission danh dau mau la failed (van giu lai de truy vet) va tra loi goc.
func (h *BotHandler) failSubmission(ctx context.Context, res *bot.Result, cause error) error {
	e := apperr.From(cause)
	_ = h.tasks.store.Bots().UpdateSubmissionStatus(ctx, res.Submission.ID, domain.SubmissionFailed, e.Message)
	return cause
}

// ---------------- doc payload jsonb an toan ----------------

func payloadString(p domain.JSONMap, keys ...string) string {
	for _, k := range keys {
		if v, ok := p[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			default:
				return strings.TrimSpace(fmt.Sprint(v))
			}
		}
	}
	return ""
}

func payloadInt(p domain.JSONMap, keys ...string) int {
	for _, k := range keys {
		v, ok := p[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case int:
			return t
		case int64:
			return int(t)
		case float64:
			return int(t)
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
				return n
			}
		}
	}
	return 0
}

func payloadUUID(p domain.JSONMap, keys ...string) (uuid.UUID, bool) {
	s := payloadString(p, keys...)
	if s == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

func payloadTime(p domain.JSONMap, keys ...string) (*time.Time, error) {
	s := payloadString(p, keys...)
	if s == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, apperr.Invalid("invalid_deadline", "deadline phai dang YYYY-MM-DD hoac RFC3339")
}
