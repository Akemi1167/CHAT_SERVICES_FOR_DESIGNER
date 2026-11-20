package grpcserver

import (
	"time"

	"github.com/google/uuid"

	chatv1 "github.com/abc-workspace/chat-service/gen/go/chat/v1"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
)

// File nay chi lam 1 viec: doi qua lai giua domain <-> proto.
// Giu mapping tap trung o day => them/xoa field chi sua 1 cho.

func ts(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func tsPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ts(*t)
}

func idPtr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func parseUUID(s, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid_uuid", field+" khong phai UUID hop le")
	}
	return id, nil
}

func parseUUIDPtr(s, field string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := parseUUID(s, field)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func parseUUIDs(in []string, field string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		id, err := parseUUID(s, field)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func parseTimePtr(s, field string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil, apperr.Invalid("invalid_time", field+" phai dang RFC3339")
	}
	return &t, nil
}

func pageFrom(p *chatv1.CursorPage) paging.Page {
	if p == nil {
		return paging.Normalize("", 0)
	}
	return paging.Normalize(p.GetCursor(), p.GetLimit())
}

func pageInfoTo(i paging.PageInfo) *chatv1.PageInfo {
	return &chatv1.PageInfo{NextCursor: i.NextCursor, HasMore: i.HasMore}
}

// ---- enums ----

func convTypeTo(t domain.ConversationType) chatv1.ConversationType {
	switch t {
	case domain.ConversationDirect:
		return chatv1.ConversationType_CONVERSATION_TYPE_DIRECT
	case domain.ConversationGroup:
		return chatv1.ConversationType_CONVERSATION_TYPE_GROUP
	default:
		return chatv1.ConversationType_CONVERSATION_TYPE_UNSPECIFIED
	}
}

func convTypeFrom(t chatv1.ConversationType) domain.ConversationType {
	switch t {
	case chatv1.ConversationType_CONVERSATION_TYPE_DIRECT:
		return domain.ConversationDirect
	case chatv1.ConversationType_CONVERSATION_TYPE_GROUP:
		return domain.ConversationGroup
	default:
		return ""
	}
}

func roleTo(r domain.MemberRole) chatv1.MemberRole {
	switch r {
	case domain.MemberOwner:
		return chatv1.MemberRole_MEMBER_ROLE_OWNER
	case domain.MemberAdmin:
		return chatv1.MemberRole_MEMBER_ROLE_ADMIN
	case domain.MemberNormal:
		return chatv1.MemberRole_MEMBER_ROLE_MEMBER
	default:
		return chatv1.MemberRole_MEMBER_ROLE_UNSPECIFIED
	}
}

func roleFrom(r chatv1.MemberRole) domain.MemberRole {
	switch r {
	case chatv1.MemberRole_MEMBER_ROLE_OWNER:
		return domain.MemberOwner
	case chatv1.MemberRole_MEMBER_ROLE_ADMIN:
		return domain.MemberAdmin
	case chatv1.MemberRole_MEMBER_ROLE_MEMBER:
		return domain.MemberNormal
	default:
		return ""
	}
}

var msgTypeToProto = map[domain.MessageType]chatv1.MessageType{
	domain.MessageText:       chatv1.MessageType_MESSAGE_TYPE_TEXT,
	domain.MessageImage:      chatv1.MessageType_MESSAGE_TYPE_IMAGE,
	domain.MessageVideo:      chatv1.MessageType_MESSAGE_TYPE_VIDEO,
	domain.MessageFile:       chatv1.MessageType_MESSAGE_TYPE_FILE,
	domain.MessageGIF:        chatv1.MessageType_MESSAGE_TYPE_GIF,
	domain.MessageSystem:     chatv1.MessageType_MESSAGE_TYPE_SYSTEM,
	domain.MessageBotCommand: chatv1.MessageType_MESSAGE_TYPE_BOT_COMMAND,
	domain.MessageBotReply:   chatv1.MessageType_MESSAGE_TYPE_BOT_REPLY,
}

func msgTypeTo(t domain.MessageType) chatv1.MessageType {
	if v, ok := msgTypeToProto[t]; ok {
		return v
	}
	return chatv1.MessageType_MESSAGE_TYPE_UNSPECIFIED
}

func msgTypeFrom(t chatv1.MessageType) domain.MessageType {
	for k, v := range msgTypeToProto {
		if v == t {
			return k
		}
	}
	return ""
}

func senderTypeTo(t domain.SenderType) chatv1.SenderType {
	switch t {
	case domain.SenderUser:
		return chatv1.SenderType_SENDER_TYPE_USER
	case domain.SenderBot:
		return chatv1.SenderType_SENDER_TYPE_BOT
	case domain.SenderSystem:
		return chatv1.SenderType_SENDER_TYPE_SYSTEM
	default:
		return chatv1.SenderType_SENDER_TYPE_UNSPECIFIED
	}
}

// ---- entities ----

func userTo(u *domain.User) *chatv1.User {
	if u == nil {
		return nil
	}
	return &chatv1.User{
		Id:          u.ID.String(),
		ExternalId:  u.ExternalID,
		DisplayName: u.DisplayName,
		AvatarUrl:   u.AvatarURL,
		Email:       u.Email,
		Role:        u.Role,
		Presence:    string(u.LivePresence()),
	}
}

func attachmentTo(a domain.Attachment) *chatv1.Attachment {
	return &chatv1.Attachment{
		Id:         a.ID.String(),
		Kind:       a.Kind,
		Url:        a.URL,
		Name:       a.Name,
		MimeType:   a.MimeType,
		SizeBytes:  a.SizeBytes,
		Width:      a.Width,
		Height:     a.Height,
		DurationMs: a.DurationMs,
	}
}

func attachmentsTo(in []domain.Attachment) []*chatv1.Attachment {
	out := make([]*chatv1.Attachment, 0, len(in))
	for _, a := range in {
		out = append(out, attachmentTo(a))
	}
	return out
}

func attachmentsFrom(in []*chatv1.Attachment) []domain.Attachment {
	out := make([]domain.Attachment, 0, len(in))
	for _, a := range in {
		if a == nil || a.GetUrl() == "" {
			continue
		}
		out = append(out, domain.Attachment{
			ID:         uuid.New(),
			Kind:       a.GetKind(),
			URL:        a.GetUrl(),
			Name:       a.GetName(),
			MimeType:   a.GetMimeType(),
			SizeBytes:  a.GetSizeBytes(),
			Width:      a.GetWidth(),
			Height:     a.GetHeight(),
			DurationMs: a.GetDurationMs(),
		})
	}
	return out
}

func messageTo(m *domain.Message) *chatv1.Message {
	if m == nil {
		return nil
	}
	return &chatv1.Message{
		Id:             m.ID.String(),
		ConversationId: m.ConversationID.String(),
		SenderId:       idPtr(m.SenderID),
		SenderType:     senderTypeTo(m.SenderType),
		Type:           msgTypeTo(m.Type),
		Content:        m.Content,
		ThreadId:       idPtr(m.ThreadID),
		ReplyToId:      idPtr(m.ReplyToID),
		MetadataJson:   m.Metadata.String(),
		Attachments:    attachmentsTo(m.Attachments),
		CreatedAt:      ts(m.CreatedAt),
		UpdatedAt:      ts(m.UpdatedAt),
		ClientMsgId:    m.ClientMsgID,
		Sender:         userTo(m.Sender),
	}
}

func messagesTo(in []domain.Message) []*chatv1.Message {
	out := make([]*chatv1.Message, 0, len(in))
	for i := range in {
		out = append(out, messageTo(&in[i]))
	}
	return out
}

func memberTo(m domain.ConversationMember) *chatv1.ConversationMember {
	return &chatv1.ConversationMember{
		UserId:            m.UserID.String(),
		Role:              roleTo(m.Role),
		LastReadMessageId: idPtr(m.LastReadMessageID),
		Muted:             m.Muted,
		User:              userTo(m.User),
	}
}

func conversationTo(c *domain.Conversation) *chatv1.Conversation {
	if c == nil {
		return nil
	}
	members := make([]*chatv1.ConversationMember, 0, len(c.Members))
	for _, m := range c.Members {
		members = append(members, memberTo(m))
	}
	return &chatv1.Conversation{
		Id:            c.ID.String(),
		Type:          convTypeTo(c.Type),
		Name:          c.Name,
		AvatarUrl:     c.AvatarURL,
		CreatedBy:     c.CreatedBy.String(),
		LastMessageAt: tsPtr(c.LastMessageAt),
		UnreadCount:   c.UnreadCount,
		Members:       members,
	}
}

func botTo(b domain.Bot) *chatv1.Bot {
	return &chatv1.Bot{
		Id: b.ID.String(), Slug: b.Slug, Name: b.Name,
		Description: b.Description, Enabled: b.Enabled,
	}
}

func botTemplateTo(slug string, t domain.BotTemplate) *chatv1.BotTemplate {
	return &chatv1.BotTemplate{
		Id: t.ID.String(), BotSlug: slug, Code: t.Code, Name: t.Name,
		Description: t.Description, SchemaJson: t.Schema.String(), Enabled: t.Enabled,
	}
}

func submissionTo(s *domain.BotSubmission) *chatv1.BotSubmission {
	if s == nil {
		return nil
	}
	return &chatv1.BotSubmission{
		Id:             s.ID.String(),
		BotSlug:        s.BotSlug,
		TemplateCode:   s.TemplateCode,
		ConversationId: s.ConversationID.String(),
		MessageId:      idPtr(s.MessageID),
		ThreadId:       idPtr(s.ThreadID),
		UserId:         s.UserID.String(),
		RawText:        s.RawText,
		PayloadJson:    s.Payload.String(),
		Status:         string(s.Status),
		Error:          s.Error,
		CreatedAt:      ts(s.CreatedAt),
	}
}
