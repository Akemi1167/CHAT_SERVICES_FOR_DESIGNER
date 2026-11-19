package ws

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

type Handler struct {
	cfg   config.HTTP
	hub   *realtime.Hub
	msgs  *message.Service
	log   *slog.Logger
	store repository.Store
}

func NewHandler(cfg config.HTTP, hub *realtime.Hub, msgs *message.Service, store repository.Store) *Handler {
	return &Handler{cfg: cfg, hub: hub, msgs: msgs, log: slog.Default().With("component", "ws"), store: store}
}

// Upgrade chan request khong phai WebSocket (dat SAU middleware Auth).
func (h *Handler) Upgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return fiber.ErrUpgradeRequired
	}
	// Chuyen identity sang websocket.Conn.Locals.
	id, err := middleware.Identity(c)
	if err != nil {
		return err
	}
	c.Locals(middleware.IdentityKey, id)
	return c.Next()
}

// Handle la handler chinh cua ket noi WebSocket.
//
// dialect quyet dinh hinh dang frame day xuong:
//
//	realtime.DialectInternal  /ws                     snake_case (ban goc)
//	realtime.DialectClient    /api/client/v1/chat/ws  camelCase theo hop dong CMS
func (h *Handler) Handle(dialect realtime.Dialect) fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		identity, ok := conn.Locals(middleware.IdentityKey).(*auth.Identity)
		if !ok || identity == nil {
			_ = conn.WriteJSON(Outbound{Type: OutError, Payload: errorPayload{
				Code: "unauthenticated", Message: "Thieu hoac sai token",
			}})
			_ = conn.Close()
			return
		}

		// Context song cung ket noi, mang theo identity cho service layer.
		ctx, cancel := context.WithCancel(auth.WithIdentity(context.Background(), identity))
		defer cancel()

		client := realtime.NewClient(identity.UserID, h.cfg.WSSendBuffer)
		client.Dialect = dialect
		client.ActorID = identity.ActorID
		h.hub.Register(ctx, client)
		defer h.hub.Unregister(ctx, client)

		conn.SetReadLimit(h.cfg.WSReadLimitByte)
		readTimeout := h.cfg.WSReadTimeout
		if readTimeout <= h.cfg.WSPingInterval {
			readTimeout = 2 * h.cfg.WSPingInterval
		}
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
		// Client chuan se tu tra pong; moi lan nhan pong hoac frame thi gia han.
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(readTimeout))
		})

		// 1 goroutine ghi duy nhat (gorilla/websocket khong cho ghi dong thoi).
		writerDone := make(chan struct{})
		go h.writePump(conn, client, writerDone)

		// Tra ro "tab nay dang dang nhap bang ai" => nhin frame ready la biet ngay
		// co dung nguoi hay khong (hay gap khi dan sai token/tool gan san header).
		ready := fiber.Map{
			"user_id":      identity.UserID,
			"external_id":  identity.ExternalID,
			"display_name": identity.Name,
			"role":         identity.Role,
			"conn_id":      client.ID,
			"server_now":   time.Now().UTC(),
		}
		if dialect == realtime.DialectClient {
			// Be mat client dung camelCase + actorId thay vi external_id noi bo.
			ready = fiber.Map{
				"userId":    identity.UserID,
				"actorId":   identity.ActorID,
				"name":      identity.Name,
				"role":      identity.Role,
				"connId":    client.ID,
				"serverNow": time.Now().UTC(),
			}
		}
		h.hub.SendToClient(client, Outbound{Type: OutReady, Payload: ready})
		h.log.Info("ws connected",
			"user_id", identity.UserID, "name", identity.Name,
			"external_id", identity.ExternalID, "conn_id", client.ID)

		// Read loop.
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				if os.IsTimeout(err) {
					// Client khong tra pong va cung khong gui gi trong readTimeout.
					h.log.Info("dong ket noi WS vi khong co tin hieu song",
						"user_id", identity.UserID, "conn_id", client.ID,
						"read_timeout", readTimeout.String())
				} else if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					h.log.Debug("ws read error", "user_id", identity.UserID, "err", err)
				}
				break
			}
			_ = conn.SetReadDeadline(time.Now().Add(readTimeout))

			var in Inbound
			if err := json.Unmarshal(raw, &in); err != nil {
				h.fail(client, "", apperr.Invalid("invalid_frame", "Frame khong phai JSON hop le"))
				continue
			}
			h.dispatch(ctx, client, identity, in)
		}

		client.Close()
		<-writerDone
	})
}

func (h *Handler) writePump(conn *websocket.Conn, client *realtime.Client, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(h.cfg.WSPingInterval)
	defer ticker.Stop()

	for {
		select {
		case frame, ok := <-client.Outbound():
			if !ok {
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
					time.Now().Add(time.Second))
				_ = conn.Close()
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(h.cfg.WSWriteTimeout))
			if err := conn.WriteMessage(websocket.TextMessage, frame); err != nil {
				_ = conn.Close()
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(h.cfg.WSWriteTimeout))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				_ = conn.Close()
				return
			}
		}
	}
}

// dispatch route frame -> service. Them loai frame moi = them 1 case.
func (h *Handler) dispatch(ctx context.Context, client *realtime.Client, identity *auth.Identity, in Inbound) {
	switch in.Type {
	case InPing:
		h.hub.SendToClient(client, Outbound{Type: OutPong, Ref: in.ID,
			Payload: fiber.Map{"server_now": time.Now().UTC()}})

	case InMessageSend:
		var p sendMessagePayload
		if err := json.Unmarshal(in.Payload, &p); err != nil {
			h.fail(client, in.ID, apperr.Invalid("invalid_payload", "payload khong hop le"))
			return
		}
		p.normalize()
		convID, err := h.resolveConvID(ctx, p.ConversationID)
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		threadID, err := optUUID(p.ThreadID, "thread_id")
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		replyTo, err := optUUID(p.ReplyToID, "reply_to_id")
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		res, err := h.msgs.Send(ctx, message.SendInput{
			ActorID:        identity.UserID,
			ConversationID: convID,
			Type:           domain.MessageType(p.Type),
			Content:        p.Content,
			ThreadID:       threadID,
			ReplyToID:      replyTo,
			Metadata:       p.Metadata,
			Attachments:    toDomainAttachments(p.Attachments),
			ClientMsgID:    p.ClientMsgID,
		})
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		// ACK cho dung client da gui (event message.new da broadcast cho ca nhom).
		h.hub.SendToClient(client, Outbound{Type: OutAck, Ref: in.ID, Payload: fiber.Map{
			"client_msg_id": p.ClientMsgID,
			"message":       res.Message,
			"bot_reply":     res.BotReply,
			"submission":    res.Submission,
			"duplicated":    res.Duplicated,
		}})

	case InTemplateSend:
		var p templateSendPayload
		if err := json.Unmarshal(in.Payload, &p); err != nil {
			h.fail(client, in.ID, apperr.Invalid("invalid_payload", "payload khong hop le"))
			return
		}
		p.normalize()
		convID, err := h.resolveConvID(ctx, p.ConversationID)
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		threadID, err := optUUID(p.ThreadID, "thread_id")
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		content := p.RawText
		if content == "" {
			content = "@" + p.BotSlug + " " + p.TemplateCode
		}
		res, err := h.msgs.Send(ctx, message.SendInput{
			ActorID:         identity.UserID,
			ConversationID:  convID,
			Type:            domain.MessageBotCommand,
			Content:         content,
			ThreadID:        threadID,
			ClientMsgID:     p.ClientMsgID,
			BotSlug:         p.BotSlug,
			TemplateCode:    p.TemplateCode,
			TemplatePayload: p.Payload,
			Metadata: domain.JSONMap{
				"bot_slug":      p.BotSlug,
				"template_code": p.TemplateCode,
				"payload":       map[string]any(p.Payload),
			},
		})
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		h.hub.SendToClient(client, Outbound{Type: OutAck, Ref: in.ID, Payload: fiber.Map{
			"message":    res.Message,
			"bot_reply":  res.BotReply,
			"submission": res.Submission,
		}})

	case InMessageRead:
		var p markReadPayload
		if err := json.Unmarshal(in.Payload, &p); err != nil {
			h.fail(client, in.ID, apperr.Invalid("invalid_payload", "payload khong hop le"))
			return
		}
		p.normalize()
		convID, err := h.resolveConvID(ctx, p.ConversationID)
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		msgID, err := uuid.Parse(p.MessageID)
		if err != nil {
			h.fail(client, in.ID, apperr.Invalid("invalid_uuid", "message_id phai la UUID"))
			return
		}
		unread, err := h.msgs.MarkRead(ctx, identity.UserID, convID, msgID)
		if err != nil {
			h.fail(client, in.ID, err)
			return
		}
		h.hub.SendToClient(client, Outbound{Type: OutAck, Ref: in.ID,
			Payload: fiber.Map{"unread_count": unread}})

	case InMessageDelete:
		var p deletePayload
		if err := json.Unmarshal(in.Payload, &p); err != nil {
			h.fail(client, in.ID, apperr.Invalid("invalid_payload", "payload khong hop le"))
			return
		}
		p.normalize()
		msgID, err := uuid.Parse(p.MessageID)
		if err != nil {
			h.fail(client, in.ID, apperr.Invalid("invalid_uuid", "message_id phai la UUID"))
			return
		}
		if err := h.msgs.Delete(ctx, identity.UserID, msgID); err != nil {
			h.fail(client, in.ID, err)
			return
		}
		h.hub.SendToClient(client, Outbound{Type: OutAck, Ref: in.ID, Payload: fiber.Map{"deleted": true}})

	case InTyping:
		var p typingPayload
		if err := json.Unmarshal(in.Payload, &p); err != nil {
			return // typing khong quan trong, bo qua im lang
		}
		p.normalize()
		convID, err := h.resolveConvID(ctx, p.ConversationID)
		if err != nil {
			return
		}
		if err := h.msgs.Typing(ctx, identity.UserID, convID, p.Typing); err != nil {
			h.log.Debug("typing failed", "err", err)
		}

	default:
		h.fail(client, in.ID, apperr.Invalid("unknown_frame_type", "Khong ho tro frame type: "+in.Type))
	}
}

func (h *Handler) fail(client *realtime.Client, ref string, err error) {
	e := apperr.From(err)
	h.hub.SendToClient(client, Outbound{Type: OutError, Ref: ref, Payload: errorPayload{
		Code: e.Code, Message: e.Message, Details: e.Details,
	}})
}

func optUUID(s, field string) (*uuid.UUID, error) {
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, apperr.Invalid("invalid_uuid", field+" phai la UUID")
	}
	return &id, nil
}

func toDomainAttachments(in []attachmentPayload) []domain.Attachment {
	out := make([]domain.Attachment, 0, len(in))
	for _, a := range in {
		if a.URL == "" {
			continue
		}
		out = append(out, domain.Attachment{
			ID: uuid.New(), Kind: a.Kind, URL: a.URL, Name: a.Name,
			MimeType: a.MimeType, SizeBytes: a.SizeBytes,
			Width: a.Width, Height: a.Height, DurationMs: a.DurationMs,
		})
	}
	return out
}

// resolveConvID nhan UUID noi bo HOAC chatId cua CMS (wg_<hex>), giong moi
// route hoi thoai cua REST — de FE dung chung mot gia tri id o moi noi.
func (h *Handler) resolveConvID(ctx context.Context, raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, apperr.Invalid("invalid_request", "thieu conversationId")
	}
	if id, err := uuid.Parse(raw); err == nil {
		return id, nil
	}
	if h.store == nil {
		return uuid.Nil, apperr.Invalid("invalid_uuid", "conversationId phai la UUID")
	}
	conv, err := h.store.Conversations().GetByExternalID(ctx, raw)
	if err != nil {
		return uuid.Nil, err
	}
	return conv.ID, nil
}
