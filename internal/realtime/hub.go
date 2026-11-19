package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PresenceHook duoc goi khi user online/offline (dung de update DB + broadcast).
type PresenceHook func(ctx context.Context, userID uuid.UUID, online bool)

type Hub struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]map[*Client]struct{}

	broker   Broker
	presence PresenceHook
	log      *slog.Logger

	// clientRender: doi payload sang hinh dang be mat client (xem dialect.go).
	// Chi set 1 lan luc wiring nen khong can lock.
	clientRender ClientRenderer
}

func NewHub(broker Broker) *Hub {
	return &Hub{
		clients: make(map[uuid.UUID]map[*Client]struct{}),
		broker:  broker,
		log:     slog.Default().With("component", "realtime.hub"),
	}
}

func (h *Hub) SetPresenceHook(fn PresenceHook) { h.presence = fn }

// Run lang nghe broker va day event xuong client cua instance nay.
// Neu broker dong channel (vd Redis mat ket noi) thi subscribe lai,
// khong duoc dung im lang vi nhu vay toan bo realtime se "chet" ma khong ai biet.
func (h *Hub) Run(ctx context.Context) error {
	const retryDelay = 2 * time.Second
	for {
		ch, err := h.broker.Subscribe(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			h.log.Error("subscribe broker that bai, thu lai", "err", err, "retry_in", retryDelay.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(retryDelay):
				continue
			}
		}
		h.log.Info("hub started")

		if closed := h.consume(ctx, ch); !closed {
			h.log.Info("hub stopped")
			return nil
		}
		if ctx.Err() != nil {
			h.log.Info("hub stopped")
			return nil
		}
		h.log.Error("broker dong channel, subscribe lai", "retry_in", retryDelay.String())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retryDelay):
		}
	}
}

// consume tra ve true neu channel bi dong (can subscribe lai), false neu context ket thuc.
func (h *Hub) consume(ctx context.Context, ch <-chan Event) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case ev, ok := <-ch:
			if !ok {
				return true
			}
			h.deliver(ev)
		}
	}
}

// Publish gui event ra broker (moi instance deu nhan duoc).
func (h *Hub) Publish(ctx context.Context, ev Event) {
	if err := h.broker.Publish(ctx, ev); err != nil {
		h.log.Error("publish event failed", "type", ev.Type, "err", err)
	}
}

func (h *Hub) Register(ctx context.Context, c *Client) {
	h.mu.Lock()
	set, ok := h.clients[c.UserID]
	if !ok {
		set = make(map[*Client]struct{})
		h.clients[c.UserID] = set
	}
	set[c] = struct{}{}
	first := len(set) == 1
	h.mu.Unlock()

	h.log.Debug("client registered", "user_id", c.UserID, "conn_id", c.ID)
	if first && h.presence != nil {
		h.presence(ctx, c.UserID, true)
	}
}

func (h *Hub) Unregister(ctx context.Context, c *Client) {
	h.mu.Lock()
	last := false
	if set, ok := h.clients[c.UserID]; ok {
		delete(set, c)
		if len(set) == 0 {
			delete(h.clients, c.UserID)
			last = true
		}
	}
	h.mu.Unlock()

	c.Close()
	h.log.Debug("client unregistered", "user_id", c.UserID, "conn_id", c.ID)
	if last && h.presence != nil {
		h.presence(ctx, c.UserID, false)
	}
}

func (h *Hub) IsOnline(userID uuid.UUID) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	_, ok := h.clients[userID]
	return ok
}

// Connections tra so ket noi dang mo theo tung user (chi cua instance nay).
// Dung cho endpoint debug: "tab nay con song khong?".
func (h *Hub) Connections() map[uuid.UUID]int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[uuid.UUID]int, len(h.clients))
	for id, set := range h.clients {
		out[id] = len(set)
	}
	return out
}

// OnlineUserIDs chi tra user dang connect vao instance nay.
func (h *Hub) OnlineUserIDs() []uuid.UUID {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]uuid.UUID, 0, len(h.clients))
	for id := range h.clients {
		out = append(out, id)
	}
	return out
}

// frameInternal: hinh dang cu cua /ws — snake_case, payload nguyen ban.
func (h *Hub) frameInternal(ev Event) ([]byte, error) {
	return json.Marshal(map[string]any{
		"type":            string(ev.Type),
		"event_id":        ev.ID,
		"conversation_id": ev.ConversationID,
		"payload":         ev.Payload,
		"ts":              ev.CreatedAt,
	})
}

// frameClient: hinh dang cua /api/client/v1/chat/ws — camelCase, payload da doi
// qua clientview. Chua tiem renderer thi roi ve frame cu (khong mat event).
func (h *Hub) frameClient(ev Event, actorID string) ([]byte, error) {
	if h.clientRender == nil {
		return h.frameInternal(ev)
	}
	return json.Marshal(map[string]any{
		"type":           string(ev.Type),
		"eventId":        ev.ID,
		"conversationId": ev.ConversationID,
		"payload":        h.clientRender(ev.Payload, actorID),
		"ts":             ev.CreatedAt,
	})
}

// SendToClient day 1 frame cho dung 1 ket noi (dung cho ACK/error cua chinh request do).
func (h *Hub) SendToClient(c *Client, frame any) {
	raw, err := json.Marshal(frame)
	if err != nil {
		h.log.Error("marshal frame failed", "err", err)
		return
	}
	if !c.push(raw) {
		h.log.Warn("client send buffer full, dong ket noi", "user_id", c.UserID, "conn_id", c.ID)
		c.Close()
	}
}

func (h *Hub) deliver(ev Event) {
	raw, err := h.frameInternal(ev)
	if err != nil {
		h.log.Error("marshal event failed", "type", ev.Type, "err", err)
		return
	}
	// Frame cho be mat client dung rieng theo tung actorId (card loc theo nguoi
	// xem), nen dung lai trong 1 lan deliver thay vi marshal lai moi ket noi.
	clientFrames := map[string][]byte{}

	delivered, offline := 0, make([]string, 0, len(ev.Recipients))
	for _, uid := range ev.Recipients {
		if ev.SkipUserID != nil && *ev.SkipUserID == uid {
			continue
		}
		h.mu.RLock()
		set := h.clients[uid]
		targets := make([]*Client, 0, len(set))
		for c := range set {
			targets = append(targets, c)
		}
		h.mu.RUnlock()

		if len(targets) == 0 {
			// Khong online o instance nay: binh thuong khi chay nhieu instance,
			// nhung neu chay 1 instance thi nghia la nguoi do khong ket noi.
			offline = append(offline, uid.String())
			continue
		}
		for _, c := range targets {
			out := raw
			if c.Dialect == DialectClient {
				b, ok := clientFrames[c.ActorID]
				if !ok {
					var cerr error
					if b, cerr = h.frameClient(ev, c.ActorID); cerr != nil {
						h.log.Error("marshal event (client) failed",
							"type", ev.Type, "actor_id", c.ActorID, "err", cerr)
						continue
					}
					clientFrames[c.ActorID] = b
				}
				out = b
			}
			if !c.push(out) {
				h.log.Warn("drop slow client", "user_id", uid, "conn_id", c.ID)
				c.Close()
				continue
			}
			delivered++
		}
	}

	// Log nay tra loi cau hoi hay gap nhat: "tin nhan co duoc day xuong ai khong?"
	h.log.Debug("deliver event",
		"type", string(ev.Type),
		"conversation_id", ev.ConversationID,
		"recipients", len(ev.Recipients),
		"delivered_connections", delivered,
		"not_connected_here", offline)
}
