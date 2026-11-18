package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

// WSStatusHandler tra lai ai dang mo WebSocket tren INSTANCE hien tai.
// Dung khi test: truoc khi gui tin, kiem tra tab cua nguoi nhan con song khong.
// Chay nhieu instance thi moi instance chi thay ket noi cua chinh no.
type WSStatusHandler struct {
	hub   *realtime.Hub
	store repository.Store
}

func NewWSStatusHandler(hub *realtime.Hub, store repository.Store) *WSStatusHandler {
	return &WSStatusHandler{hub: hub, store: store}
}

type onlineUser struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name,omitempty"`
	Connections int    `json:"connections"`
}

func (h *WSStatusHandler) Online(c *fiber.Ctx) error {
	me, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}

	conns := h.hub.Connections()
	ids := make([]uuid.UUID, 0, len(conns))
	for id := range conns {
		ids = append(ids, id)
	}
	users, err := h.store.Users().ListByIDs(c.UserContext(), ids)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	names := make(map[string]string, len(users))
	for _, u := range users {
		names[u.ID.String()] = u.DisplayName
	}

	out := make([]onlineUser, 0, len(conns))
	total := 0
	for id, n := range conns {
		total += n
		out = append(out, onlineUser{UserID: id.String(), DisplayName: names[id.String()], Connections: n})
	}

	return httpresp.OK(c, fiber.Map{
		"instance_total_connections": total,
		"online_users":               out,
		"me_connected":               conns[me.UserID] > 0,
		"my_connections":             conns[me.UserID],
	})
}
