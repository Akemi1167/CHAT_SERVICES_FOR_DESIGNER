package card

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
)

// MessagePoster la cua de ghi dong lich su vao hoi thoai (message.Service cai dat).
type MessagePoster interface {
	PostSystem(ctx context.Context, in message.SystemMessageInput) (*domain.Message, error)
}

// Service noi UI <-> CMS cho 2 thao tac dong bo: gui form va bam nut tren card.
type Service struct {
	store   repository.Store
	convs   *conversation.Service
	client  *cms.Client
	hub     *realtime.Hub
	poster  MessagePoster
	timeout time.Duration
	log     *slog.Logger
}

func New(store repository.Store, convs *conversation.Service, client *cms.Client, hub *realtime.Hub, poster MessagePoster, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Service{
		store: store, convs: convs, client: client, hub: hub, poster: poster, timeout: timeout,
		log: slog.Default().With("component", "service.card"),
	}
}

// Result la ket qua CMS tra ve, da chuan hoa cho UI.
type Result struct {
	OK          bool           `json:"ok"`
	Toast       *cms.Toast     `json:"toast,omitempty"`
	CloseForm   bool           `json:"closeForm"`
	Card        domain.JSONMap `json:"card,omitempty"` // da loc theo nguoi bam
	CardVersion int            `json:"cardVersion,omitempty"`
	MessageID   *uuid.UUID     `json:"messageId,omitempty"`
}

type SubmitFormInput struct {
	ActorID        uuid.UUID
	ConversationID uuid.UUID
	ThreadID       *uuid.UUID
	FormID         string
	Values         domain.JSONMap
}

// SubmitForm gui form (vd create_task) sang CMS va tra ket qua cho UI.
// FE gui team_id trong values — Chat khong dien giai, chuyen nguyen sang CMS.
func (s *Service) SubmitForm(ctx context.Context, in SubmitFormInput) (*Result, error) {
	if !s.client.Enabled() {
		return nil, apperr.Invalid("cms_disabled", "Chua bat tich hop CMS (CMS_ENABLED)")
	}
	if in.FormID == "" {
		return nil, apperr.Invalid("form_id_required", "Thieu formId")
	}

	actor, conv, err := s.actorAndConversation(ctx, in.ActorID, in.ConversationID)
	if err != nil {
		return nil, err
	}
	// Tao yeu cau chi trong nhom Web; chat rieng voi bot chi nhan thong bao.
	if conv.IsBotDM() {
		return nil, apperr.ErrBotDMReadOnly
	}

	fields := formSubmitFields(in, botIDOf(conv), groupID(conv),
		cms.Actor(actor.ActorID, actor.OpenID, actor.CMSUserID, actor.DisplayName))

	resp, err := s.client.SendEvent(ctx, cms.EventFormSubmit, fields, s.timeout)
	if err != nil {
		s.log.Error("gui form.submit that bai", "form_id", in.FormID, "err", err)
		return nil, apperr.Internal("cms_unreachable", "Khong gui duoc yeu cau sang CMS, thu lai sau").Wrap(err)
	}

	if resp.OK {
		// Ghi lai trong lich su nhom: CMS da nhan form (card task se toi sau qua Bot API).
		meta := domain.JSONMap{
			"kind":    HistoryFormSubmitted,
			"form_id": in.FormID,
			"values":  map[string]any(in.Values),
		}
		if resp.Toast != nil {
			meta["toast"] = map[string]any{"type": resp.Toast.Type, "text": resp.Toast.Text}
		}
		s.recordHistory(ctx, message.SystemMessageInput{
			ConversationID: in.ConversationID,
			ThreadID:       in.ThreadID,
			Content:        formHistoryText(actor.DisplayName, in.FormID, resp.Toast),
			ActorID:        &in.ActorID,
			Metadata:       meta,
			// Frame rieng: FE khong ve nhu tin chat moi (card don toi ngay sau qua `message`).
			Event: realtime.EventFormSubmitted,
		})
	}
	return &Result{OK: resp.OK, Toast: resp.Toast, CloseForm: resp.CloseForm}, nil
}

// formSubmitFields dung phan rieng cua event form.submit (khung chung do cms.BuildEvent them).
// values chuyen NGUYEN (ke ca useAI) — Chat khong doc noi dung.
func formSubmitFields(in SubmitFormInput, botID, groupID string, actor map[string]any) map[string]any {
	ctx := map[string]any{"groupId": groupID}
	if in.ThreadID != nil {
		ctx["threadRootId"] = in.ThreadID.String()
	}
	fields := map[string]any{
		"botId":   botID,
		"formId":  in.FormID,
		"values":  map[string]any(in.Values),
		"actor":   actor,
		"context": ctx,
	}
	return fields
}

type ActionInput struct {
	ActorID   uuid.UUID
	MessageID uuid.UUID
	Action    string
	Values    domain.JSONMap
}

// RunAction kiem quyen tren card roi chuyen event card.action sang CMS.
// CMS tra card moi -> Chat thay card (theo cardVersion) va phat realtime cho ca nhom.
func (s *Service) RunAction(ctx context.Context, in ActionInput) (*Result, error) {
	if !s.client.Enabled() {
		return nil, apperr.Invalid("cms_disabled", "Chua bat tich hop CMS (CMS_ENABLED)")
	}

	msg, err := s.store.Messages().GetByID(ctx, in.MessageID)
	if err != nil {
		return nil, err
	}
	if msg.Card == nil {
		return nil, apperr.Invalid("message_has_no_card", "Tin nhan nay khong phai card")
	}

	actor, conv, err := s.actorAndConversation(ctx, in.ActorID, msg.ConversationID)
	if err != nil {
		return nil, err
	}

	// Chan tu Chat: action phai co trong card va actor phai nam trong visibleTo.
	if !CanAct(msg.Card, in.Action, actor.ActorID) {
		return nil, apperr.Forbidden("action_not_allowed",
			"Ban khong duoc phep thuc hien thao tac nay").
			WithDetail("action", in.Action)
	}

	// values la JSON tuy y (co the long nhau, vd split_task gui values.parts la
	// mang object) — chuyen NGUYEN sang CMS. Nut confirm khong co form thi FE
	// co the bo han values: gui {} thay vi null de CMS khong phai xu ly 2 dang.
	values := map[string]any(in.Values)
	if values == nil {
		values = map[string]any{}
	}

	fields := map[string]any{
		"botId":       botIDOf(conv),
		"messageId":   msg.ID.String(),
		"action":      in.Action,
		"values":      values,
		"actor":       cms.Actor(actor.ActorID, actor.OpenID, actor.CMSUserID, actor.DisplayName),
		"group":       map[string]any{"id": groupID(conv)},
		"cardVersion": msg.CardVersion,
	}

	resp, err := s.client.SendEvent(ctx, cms.EventCardAction, fields, s.timeout)
	if err != nil {
		s.log.Error("gui card.action that bai", "action", in.Action, "err", err)
		return nil, apperr.Internal("cms_unreachable", "Khong gui duoc thao tac sang CMS, thu lai sau").Wrap(err)
	}

	out := &Result{OK: resp.OK, Toast: resp.Toast, CloseForm: resp.CloseForm, MessageID: &msg.ID}

	if resp.OK {
		// Card duoc sua tai cho => ghi them 1 dong vao thread cua card de giu dien bien.
		// Card nam trong thread (card tung phan) thi ghi vao thread goc — thread chi 1 cap.
		threadID := msg.ID
		if msg.ThreadID != nil {
			threadID = *msg.ThreadID
		}
		s.recordHistory(ctx, message.SystemMessageInput{
			ConversationID: msg.ConversationID,
			ThreadID:       &threadID,
			Content:        fmt.Sprintf("%s · %s", actor.DisplayName, actionLabel(msg.Card, in.Action)),
			ActorID:        &in.ActorID,
			Metadata: domain.JSONMap{
				"kind":            HistoryCardAction,
				"action":          in.Action,
				"values":          values,
				"card_message_id": msg.ID.String(),
				"card_version":    msg.CardVersion,
			},
		})
	}

	newCard, err := resp.CardMap()
	if err != nil {
		s.log.Error("card CMS tra ve khong hop le", "err", err)
		return out, nil
	}
	if newCard == nil || resp.CardVersion <= msg.CardVersion {
		return out, nil
	}

	applied, err := s.store.Messages().UpdateCard(ctx, msg.ID, newCard, resp.CardVersion)
	if err != nil {
		return nil, err
	}
	if applied {
		msg.Card = newCard
		msg.CardVersion = resp.CardVersion
		s.Broadcast(ctx, msg, realtime.EventCardUpdated)
	}

	out.Card = FilterForViewer(newCard, actor.ActorID)
	out.CardVersion = resp.CardVersion
	return out, nil
}

// ApplyCardPatch dung cho Bot API PATCH /bot/v1/messages/:id (CMS chu dong sua card).
func (s *Service) ApplyCardPatch(ctx context.Context, messageID uuid.UUID, newCard domain.JSONMap, version int) (bool, error) {
	applied, err := s.store.Messages().UpdateCard(ctx, messageID, newCard, version)
	if err != nil || !applied {
		return false, err
	}
	msg, err := s.store.Messages().GetByID(ctx, messageID)
	if err != nil {
		return true, err
	}
	s.Broadcast(ctx, msg, realtime.EventCardUpdated)
	return true, nil
}

// Broadcast phat tin nhan/card cho thanh vien nhom.
// Card co visibleTo nen MOI nguoi nhan mot ban da loc rieng — khong gui chung
// mot payload, trang lo nut ma nguoi do khong duoc thay.
func (s *Service) Broadcast(ctx context.Context, msg *domain.Message, evType realtime.EventType) {
	recipients, err := s.convs.MemberUserIDs(ctx, msg.ConversationID)
	if err != nil {
		s.log.Error("khong lay duoc thanh vien de phat tin", "err", err)
		return
	}
	if len(recipients) == 0 {
		return
	}

	if msg.Card == nil {
		s.hub.Publish(ctx, realtime.NewEvent(evType, msg.ConversationID.String(), recipients, msg))
		return
	}

	users, err := s.store.Users().ListByIDs(ctx, recipients)
	if err != nil {
		s.log.Error("khong lay duoc danh sach user de loc card", "err", err)
		return
	}
	for _, u := range users {
		view := *msg
		view.Card = FilterForViewer(msg.Card, u.ActorID)
		s.hub.Publish(ctx, realtime.NewEvent(
			evType, msg.ConversationID.String(), []uuid.UUID{u.ID}, &view,
		))
	}
}

// metadata.kind cua dong lich su Chat tu ghi (FE dung de hien kieu rieng neu muon).
const (
	HistoryFormSubmitted = "form_submitted"
	HistoryCardAction    = "card_action"
)

// recordHistory ghi 1 tin he thong vao nhom. Loi chi log: CMS da xu ly xong,
// khong duoc bao that bai cho UI chi vi khong ghi duoc lich su.
func (s *Service) recordHistory(ctx context.Context, in message.SystemMessageInput) {
	if s.poster == nil {
		return
	}
	if _, err := s.poster.PostSystem(ctx, in); err != nil {
		s.log.Error("ghi lich su vao nhom that bai", "kind", in.Metadata["kind"], "err", err)
	}
}

// formHistoryText: uu tien cau CMS tra trong toast (co ma yeu cau), khong co thi ghi chung chung.
func formHistoryText(name, formID string, toast *cms.Toast) string {
	if toast != nil && toast.Text != "" {
		return fmt.Sprintf("%s · %s", name, toast.Text)
	}
	return fmt.Sprintf("%s · da gui form %s", name, formID)
}

// actionLabel lay chu tren nut (text/label) de dong lich su de doc, khong co thi dung ten action.
func actionLabel(c domain.JSONMap, action string) string {
	if node, ok := FindAction(c, action); ok {
		for _, k := range []string{"text", "label"} {
			if v, ok := node[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return action
}

func (s *Service) actorAndConversation(ctx context.Context, actorID, convID uuid.UUID) (*domain.User, *domain.Conversation, error) {
	if _, err := s.convs.EnsureMember(ctx, convID, actorID); err != nil {
		return nil, nil, err
	}
	actor, err := s.store.Users().GetByID(ctx, actorID)
	if err != nil {
		return nil, nil, err
	}
	conv, err := s.store.Conversations().GetByID(ctx, convID)
	if err != nil {
		return nil, nil, err
	}
	return actor, conv, nil
}

// botIDOf: event mang botId dang "b_<id>" theo spec (CMS khong dung, chi de doi chieu log).
func botIDOf(c *domain.Conversation) string {
	if c.CMSInfo != nil {
		switch v := c.CMSInfo["botId"].(type) {
		case float64:
			return fmt.Sprintf("b_%d", int64(v))
		case int64:
			return fmt.Sprintf("b_%d", v)
		case string:
			if v != "" {
				return "b_" + v
			}
		}
	}
	return ""
}

// groupID: CMS nhan nhom theo chatId (wg_...). Nhom tao trong Chat thi gui id noi bo.
func groupID(c *domain.Conversation) string {
	if c.ExternalID != nil && *c.ExternalID != "" {
		return *c.ExternalID
	}
	return c.ID.String()
}
