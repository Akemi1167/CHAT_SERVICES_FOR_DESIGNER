package domain

// ConversationType: chat rieng (direct) hoac chat nhom (group).
type ConversationType string

const (
	ConversationDirect ConversationType = "direct"
	ConversationGroup  ConversationType = "group"
)

func (t ConversationType) Valid() bool {
	return t == ConversationDirect || t == ConversationGroup
}

// MemberRole: quyen trong 1 conversation.
type MemberRole string

const (
	MemberOwner  MemberRole = "owner"
	MemberAdmin  MemberRole = "admin"
	MemberNormal MemberRole = "member"
)

func (r MemberRole) Valid() bool {
	switch r {
	case MemberOwner, MemberAdmin, MemberNormal:
		return true
	}
	return false
}

// MessageType: loai noi dung tin nhan.
type MessageType string

const (
	MessageText       MessageType = "text"
	MessageImage      MessageType = "image"
	MessageVideo      MessageType = "video"
	MessageFile       MessageType = "file"
	MessageGIF        MessageType = "gif"
	MessageSystem     MessageType = "system"
	MessageBotCommand MessageType = "bot_command"
	MessageBotReply   MessageType = "bot_reply"
)

func (t MessageType) Valid() bool {
	switch t {
	case MessageText, MessageImage, MessageVideo, MessageFile, MessageGIF,
		MessageSystem, MessageBotCommand, MessageBotReply:
		return true
	}
	return false
}

// SenderType: ai gui tin nhan.
type SenderType string

const (
	SenderUser   SenderType = "user"
	SenderBot    SenderType = "bot"
	SenderSystem SenderType = "system"
)

// Presence: trang thai nhan su (lien ket lich OFF o phase sau).
type Presence string

const (
	PresenceOnline  Presence = "online"
	PresenceOffline Presence = "offline"
	PresenceOffDuty Presence = "off_duty"
)

// SubmissionStatus: vong doi cua 1 lan "gui mau" cho bot.
type SubmissionStatus string

const (
	SubmissionStored     SubmissionStatus = "stored"
	SubmissionProcessing SubmissionStatus = "processing"
	SubmissionDone       SubmissionStatus = "done"
	SubmissionFailed     SubmissionStatus = "failed"
)
