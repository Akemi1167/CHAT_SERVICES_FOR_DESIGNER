// Package clientview dung hinh dang JSON cua be mat /api/client/v1/chat/* theo
// hop dong voi CMS: camelCase, chi phoi ra truong UI can.
//
// Ca REST (clientapi) va realtime (WebSocket tren be mat nay) deu goi package
// nay, de hai duong khong bao gio lech ten truong.
package clientview

import (
	"strings"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/service/card"
)

// User: ho so nguoi dung cho UI.
//
// CO Y khong tra: external_id, status, synced_at, is_designer, last_seen_at,
// created_at/updated_at — do la truong noi bo cua Chat, khong thuoc hop dong.
func User(u *domain.User) map[string]any {
	if u == nil {
		return nil
	}
	view := map[string]any{
		"id":        u.ID.String(),
		"actorId":   u.ActorID,
		"name":      u.DisplayName,
		"avatarUrl": u.AvatarURL,
		"role":      u.DisplayRole(),
		"presence":  string(u.Presence),
		"openId":    u.OpenID,
		"email":     u.Email,
		"username":  u.Username,
	}
	if u.CMSUserID != nil {
		view["cmsUserId"] = *u.CMSUserID
	}
	if u.CMSProfile != nil {
		view["profile"] = map[string]any(u.CMSProfile)
	}
	return view
}

// Message: 1 tin nhan. actorID = nguoi DANG XEM, dung de loc nut cua card
// theo visibleTo va liet ke action nguoi do duoc bam.
func Message(m *domain.Message, actorID string) map[string]any {
	if m == nil {
		return nil
	}
	view := map[string]any{
		"id":             m.ID.String(),
		"conversationId": m.ConversationID.String(),
		"type":           string(m.Type),
		"senderType":     string(m.SenderType),
		"text":           m.Content,
		"createdAt":      m.CreatedAt,
		"clientMsgId":    m.ClientMsgID,
	}
	if m.SenderID != nil {
		view["senderId"] = m.SenderID.String()
	}
	if m.Sender != nil {
		view["sender"] = User(m.Sender)
	}
	if m.ThreadID != nil {
		view["threadRootId"] = m.ThreadID.String()
	}
	if len(m.Attachments) > 0 {
		view["attachments"] = m.Attachments
	}
	if m.BotSender != nil {
		view["bot"] = map[string]any(m.BotSender)
	}
	if m.Metadata != nil {
		view["metadata"] = map[string]any(m.Metadata)
	}
	if m.ReplyToID != nil {
		view["replyToId"] = m.ReplyToID.String()
	}
	if m.ReplyCount > 0 {
		view["replyCount"] = m.ReplyCount
	}
	if m.ThreadCount > 0 {
		view["threadCount"] = m.ThreadCount
	}
	if m.Card != nil {
		view["card"] = map[string]any(card.FilterForViewer(m.Card, actorID))
		view["cardVersion"] = m.CardVersion
		view["actions"] = card.Actions(m.Card, actorID)
	}
	return view
}

// Messages: danh sach tin nhan, cung goc nhin.
func Messages(items []domain.Message, actorID string) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for i := range items {
		out = append(out, Message(&items[i], actorID))
	}
	return out
}

// Notification: 1 thong bao.
func Notification(n *domain.Notification) map[string]any {
	if n == nil {
		return nil
	}
	view := map[string]any{
		"id":        n.ID.String(),
		"text":      n.Text,
		"read":      n.Read,
		"createdAt": n.CreatedAt,
	}
	if n.MessageID != nil {
		view["messageId"] = n.MessageID.String()
	}
	if n.Payload != nil {
		view["payload"] = map[string]any(n.Payload)
	}
	if n.ReadAt != nil {
		view["readAt"] = *n.ReadAt
	}
	return view
}

// Page: pageInfo cua be mat nay.
func Page(nextCursor string, hasMore bool) map[string]any {
	return map[string]any{"nextCursor": nextCursor, "hasMore": hasMore}
}

// Payload doi payload cua 1 event realtime sang hinh dang cua be mat nay.
// Dung lam ClientRenderer cho realtime.Hub (xem router.go).
//
// actorID = nguoi nhan frame, de card duoc loc dung goc nhin.
func Payload(payload any, actorID string) any {
	switch p := payload.(type) {
	case nil:
		return nil
	case *domain.Message:
		return Message(p, actorID)
	case domain.Message:
		return Message(&p, actorID)
	case *domain.Notification:
		return Notification(p)
	case domain.Notification:
		return Notification(&p)
	case map[string]any:
		// Payload dang map (read, typing, presence, reload) chi co khoa don gian
		// nen doi snake_case -> camelCase la du va dung.
		return CamelKeys(p)
	default:
		return p
	}
}

// CamelKeys doi khoa snake_case sang camelCase, de quy vao map/slice con.
func CamelKeys(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[camel(k)] = camelValue(v)
	}
	return out
}

func camelValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return CamelKeys(t)
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = camelValue(t[i])
		}
		return out
	default:
		return v
	}
}

// camel: "conversation_id" -> "conversationId". Khoa khong co "_" giu nguyen.
func camel(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	parts := strings.Split(s, "_")
	var b strings.Builder
	b.Grow(len(s))
	for i, p := range parts {
		if p == "" {
			continue
		}
		if i == 0 || b.Len() == 0 {
			b.WriteString(p)
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}
