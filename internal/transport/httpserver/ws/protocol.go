// Package ws cai dat giao thuc WebSocket cho FE.
//
// Ket noi:  GET /ws?token=<JWT>        (hoac header Authorization: Bearer <JWT>,
//
//	hoac Sec-WebSocket-Protocol: <JWT>)
//
// Moi frame la 1 JSON object:
//
//	Client -> Server: {"id":"<req-id>","type":"message.send","payload":{...}}
//	Server -> Client: {"type":"message.new","payload":{...},"ts":"..."}
//	                  {"type":"ack","ref":"<req-id>","payload":{...}}
//	                  {"type":"error","ref":"<req-id>","payload":{"code":"...","message":"..."}}
package ws

import (
	"encoding/json"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// Frame type client gui len.
const (
	InMessageSend   = "message.send"
	InMessageRead   = "message.read"
	InMessageDelete = "message.delete"
	InTemplateSend  = "template.send" // "gui mau" cho bot qua WS
	InTyping        = "typing"
	InPing          = "ping"
	// InPresenceList: "ai dang online?" — tra ve anh chup tai thoi diem hoi.
	// Event `presence` chi bao KHI DOI, nen can frame nay luc moi mo tab.
	InPresenceList = "presence.list"
)

// Frame type server gui xuong (ngoai cac event cua realtime.Event).
const (
	OutAck   = "ack"
	OutError = "error"
	OutPong  = "pong"
	OutReady = "ready"
)

type Inbound struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Outbound struct {
	Type    string `json:"type"`
	Ref     string `json:"ref,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

type errorPayload struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

// ---- payload cua tung loai frame ----
//
// Moi payload nhan CA HAI cach dat ten khoa:
//
//	snake_case  conversation_id, client_msg_id, thread_id, reply_to_id, content
//	camelCase   conversationId,  clientMsgId,  threadRootId, replyToId,  text
//
// Ly do: /ws (cong cu noi bo) dung snake_case, con /api/client/v1/chat/ws dung
// camelCase giong REST cua be mat do. Mot struct nhan ca hai de FE khong phai
// doi ten khoa khi chuyen duong, va de mau cu khong vo.
// Khoa camelCase thang khi ca hai cung xuat hien.

type sendMessagePayload struct {
	ConversationID  string              `json:"conversation_id"`
	ConversationID2 string              `json:"conversationId"`
	GroupID         string              `json:"groupId"`
	Type            string              `json:"type"`
	Content         string              `json:"content"`
	Text            string              `json:"text"`
	ThreadID        string              `json:"thread_id"`
	ThreadRootID    string              `json:"threadRootId"`
	ReplyToID       string              `json:"reply_to_id"`
	ReplyToID2      string              `json:"replyToId"`
	Metadata        domain.JSONMap      `json:"metadata"`
	Attachments     []attachmentPayload `json:"attachments"`
	ClientMsgID     string              `json:"client_msg_id"`
	ClientMsgID2    string              `json:"clientMsgId"`
	// Nhac ten @ (giong REST): actorId nguoi duoc nhac; @all chi admin / tro ly.
	Mentions    []string `json:"mentions"`
	MentionAll  bool     `json:"mention_all"`
	MentionAll2 bool     `json:"mentionAll"`
}

// normalize gop 2 cach dat ten ve 1 field duy nhat.
func (p *sendMessagePayload) normalize() {
	p.ConversationID = firstNonEmpty(p.GroupID, p.ConversationID2, p.ConversationID)
	p.Content = firstNonEmpty(p.Text, p.Content)
	p.ThreadID = firstNonEmpty(p.ThreadRootID, p.ThreadID)
	p.ReplyToID = firstNonEmpty(p.ReplyToID2, p.ReplyToID)
	p.ClientMsgID = firstNonEmpty(p.ClientMsgID2, p.ClientMsgID)
	p.MentionAll = p.MentionAll || p.MentionAll2
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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

type templateSendPayload struct {
	ConversationID  string         `json:"conversation_id"`
	ConversationID2 string         `json:"conversationId"`
	GroupID         string         `json:"groupId"`
	ThreadID        string         `json:"thread_id"`
	ThreadRootID    string         `json:"threadRootId"`
	BotSlug         string         `json:"bot_slug"`
	TemplateCode    string         `json:"template_code"`
	Payload         domain.JSONMap `json:"payload"`
	RawText         string         `json:"raw_text"`
	ClientMsgID     string         `json:"client_msg_id"`
	ClientMsgID2    string         `json:"clientMsgId"`
}

func (p *templateSendPayload) normalize() {
	p.ConversationID = firstNonEmpty(p.GroupID, p.ConversationID2, p.ConversationID)
	p.ThreadID = firstNonEmpty(p.ThreadRootID, p.ThreadID)
	p.ClientMsgID = firstNonEmpty(p.ClientMsgID2, p.ClientMsgID)
}

type markReadPayload struct {
	ConversationID  string `json:"conversation_id"`
	ConversationID2 string `json:"conversationId"`
	GroupID         string `json:"groupId"`
	MessageID       string `json:"message_id"`
	MessageID2      string `json:"messageId"`
}

func (p *markReadPayload) normalize() {
	p.ConversationID = firstNonEmpty(p.GroupID, p.ConversationID2, p.ConversationID)
	p.MessageID = firstNonEmpty(p.MessageID2, p.MessageID)
}

type deletePayload struct {
	MessageID  string `json:"message_id"`
	MessageID2 string `json:"messageId"`
}

func (p *deletePayload) normalize() {
	p.MessageID = firstNonEmpty(p.MessageID2, p.MessageID)
}

// presenceListPayload: de trong conversationId => lay moi nguoi dung chung it
// nhat 1 hoi thoai voi minh. Co conversationId => chi thanh vien nhom do.
type presenceListPayload struct {
	ConversationID  string `json:"conversation_id"`
	ConversationID2 string `json:"conversationId"`
	GroupID         string `json:"groupId"`
}

func (p *presenceListPayload) normalize() {
	p.ConversationID = firstNonEmpty(p.GroupID, p.ConversationID2, p.ConversationID)
}

type typingPayload struct {
	ConversationID  string `json:"conversation_id"`
	ConversationID2 string `json:"conversationId"`
	GroupID         string `json:"groupId"`
	Typing          bool   `json:"typing"`
}

func (p *typingPayload) normalize() {
	p.ConversationID = firstNonEmpty(p.GroupID, p.ConversationID2, p.ConversationID)
}
