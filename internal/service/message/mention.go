package message

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/service/notification"
)

// Notifier tao thong bao cho nguoi duoc nhac (notification.Service cai dat).
type Notifier interface {
	Create(ctx context.Context, in notification.CreateInput) (*domain.Notification, error)
}

// SetNotifier bat thong bao "X nhac den ban trong ...". Khong goi thi Chat van luu
// mentions va danh dau "@ ban", chi khong tao thong bao.
func (s *Service) SetNotifier(n Notifier) { s.notifier = n }

// mentionPlan: ket qua giai mentions cua mot tin.
type mentionPlan struct {
	keys    domain.JSONStrings // luu vao messages.mentions
	all     bool
	notify  []uuid.UUID // user can nhan thong bao (= nguoi trong keys)
	group   *domain.Conversation
	senderN string
}

// planMentions kiem quyen @all va loc mentions FE gui: chi thanh vien nhom,
// bo nguoi gui, bo trung, toi da domain.MaxMentions. Khong co gi de nhac => nil.
func (s *Service) planMentions(ctx context.Context, in SendInput, conv *domain.Conversation, member *domain.ConversationMember) (*mentionPlan, error) {
	keys, allInList := domain.NormalizeMentionInput(in.Mentions)
	wantAll := in.MentionAll || allInList
	if len(keys) == 0 && !wantAll {
		return nil, nil
	}

	sender, err := s.store.Users().GetByID(ctx, in.ActorID)
	if err != nil {
		return nil, err
	}
	if wantAll && !sender.CanMentionAll(conv, member) {
		return nil, apperr.Forbidden("mention_all_forbidden", "Chỉ admin / trợ lý được nhắc @all")
	}

	members, err := s.store.Conversations().ListMembers(ctx, in.ConversationID)
	if err != nil {
		return nil, err
	}
	stored, notify := resolveMentions(members, in.ActorID, keys, wantAll)
	if len(stored) == 0 && !wantAll {
		return nil, nil // nhac toan nguoi ngoai nhom / chinh minh => coi nhu khong nhac
	}
	return &mentionPlan{keys: stored, all: wantAll, notify: notify, group: conv, senderN: sender.DisplayName}, nil
}

// resolveMentions doi khoa FE gui (actorId hoac UUID) sang thanh vien nhom.
// all = true: moi thanh vien (tru nguoi gui), bo qua keys.
// Tra ve khoa da chuan hoa (MentionKey, theo thu tu FE gui) va user id tuong ung.
func resolveMentions(members []domain.ConversationMember, senderID uuid.UUID, keys []string, all bool) (domain.JSONStrings, []uuid.UUID) {
	byKey := make(map[string]*domain.User, len(members)*2)
	for i := range members {
		u := members[i].User
		if u == nil || u.ID == senderID {
			continue
		}
		byKey[u.ID.String()] = u
		if u.ActorID != "" {
			byKey[u.ActorID] = u
		}
	}

	var (
		stored domain.JSONStrings
		ids    []uuid.UUID
		seen   = map[uuid.UUID]bool{}
	)
	add := func(u *domain.User) {
		if u == nil || seen[u.ID] {
			return
		}
		seen[u.ID] = true
		stored = append(stored, u.MentionKey())
		ids = append(ids, u.ID)
	}

	if all {
		for i := range members {
			if u := members[i].User; u != nil && u.ID != senderID {
				add(u)
			}
		}
		return stored, ids
	}
	if len(keys) > domain.MaxMentions {
		keys = keys[:domain.MaxMentions]
	}
	for _, k := range keys {
		add(byKey[k])
	}
	return stored, ids
}

// filterMemberKeys: mentions do CMS gui kem card (Bot API) — chi giu thanh vien nhom.
func (s *Service) filterMemberKeys(ctx context.Context, convID uuid.UUID, raw []string) (domain.JSONStrings, error) {
	keys, _ := domain.NormalizeMentionInput(raw)
	if len(keys) == 0 {
		return nil, nil
	}
	members, err := s.store.Conversations().ListMembers(ctx, convID)
	if err != nil {
		return nil, err
	}
	stored, _ := resolveMentions(members, uuid.Nil, keys, false)
	return stored, nil
}

// notifyMentions: moi nguoi duoc nhac nhan 1 thong bao tro ve chinh tin do
// (FE doc threadRootId trong thong bao de mo dung thread). Loi tung nguoi chi ghi log.
func (s *Service) notifyMentions(ctx context.Context, plan *mentionPlan, msg *domain.Message) {
	if s.notifier == nil || plan == nil || len(plan.notify) == 0 {
		return
	}
	groupName := ""
	if plan.group != nil {
		groupName = plan.group.Name
	}
	text := mentionNotificationText(plan.senderN, groupName, msg.Content)
	payload := domain.JSONMap{"kind": "mention", "mentionAll": plan.all}
	for _, uid := range plan.notify {
		if _, err := s.notifier.Create(ctx, notification.CreateInput{
			UserID: uid, Text: text, MessageID: &msg.ID, Payload: payload,
		}); err != nil {
			s.log.Warn("tao thong bao nhac ten that bai", "user_id", uid, "message_id", msg.ID, "err", err)
		}
	}
}

// mentionPreviewRunes: so ky tu dau cua tin dua vao thong bao.
const mentionPreviewRunes = 80

// mentionNotificationText: "<Nguoi gui> nhắc đến bạn trong <tên nhóm>: <80 ký tự đầu>".
func mentionNotificationText(sender, group, content string) string {
	if sender == "" {
		sender = "Ai đó"
	}
	text := sender + " nhắc đến bạn"
	if group = strings.TrimSpace(group); group != "" {
		text += " trong " + group
	}
	preview := strings.Join(strings.Fields(content), " ")
	if r := []rune(preview); len(r) > mentionPreviewRunes {
		preview = string(r[:mentionPreviewRunes]) + "…"
	}
	if preview != "" {
		text += ": " + preview
	}
	return text
}
