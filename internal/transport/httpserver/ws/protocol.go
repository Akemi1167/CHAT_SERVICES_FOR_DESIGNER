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

type sendMessagePayload struct {
	ConversationID string              `json:"conversation_id"`
	Type           string              `json:"type"`
	Content        string              `json:"content"`
	ThreadID       string              `json:"thread_id"`
	ReplyToID      string              `json:"reply_to_id"`
	Metadata       domain.JSONMap      `json:"metadata"`
	Attachments    []attachmentPayload `json:"attachments"`
	ClientMsgID    string              `json:"client_msg_id"`
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
	ConversationID string         `json:"conversation_id"`
	ThreadID       string         `json:"thread_id"`
	BotSlug        string         `json:"bot_slug"`
	TemplateCode   string         `json:"template_code"`
	Payload        domain.JSONMap `json:"payload"`
	RawText        string         `json:"raw_text"`
	ClientMsgID    string         `json:"client_msg_id"`
}

type markReadPayload struct {
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
}

type deletePayload struct {
	MessageID string `json:"message_id"`
}

type typingPayload struct {
	ConversationID string `json:"conversation_id"`
	Typing         bool   `json:"typing"`
}
