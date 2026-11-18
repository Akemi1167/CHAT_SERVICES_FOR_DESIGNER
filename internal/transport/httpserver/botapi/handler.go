package botapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/card"
	"github.com/abc-workspace/chat-service/internal/service/cmssync"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/service/notification"
)

type Handler struct {
	store  repository.Store
	sync   *cmssync.Service
	msgs   *message.Service
	cards  *card.Service
	notifs *notification.Service
}

func NewHandler(
	store repository.Store,
	sync *cmssync.Service,
	msgs *message.Service,
	cards *card.Service,
	notifs *notification.Service,
) *Handler {
	return &Handler{store: store, sync: sync, msgs: msgs, cards: cards, notifs: notifs}
}

// Register gan toan bo route Bot API.
func (h *Handler) Register(router fiber.Router) {
	router.Post("/users/resolve", h.ResolveUsers)
	router.Post("/messages", h.PostMessage)
	router.Patch("/messages/:id", h.PatchMessage)
	router.Post("/messages/:id/thread", h.ThreadReply)
	router.Post("/notifications", h.CreateNotification)
	router.Put("/groups/:id", h.PutGroup)
	router.Get("/groups/:id/members", h.GroupMembers)
}

// ---------------- users/resolve ----------------

type resolveReq struct {
	Items []struct {
		OpenID    string `json:"openId"`
		CMSUserID int64  `json:"cmsUserId"`
		Name      string `json:"name"`
	} `json:"items"`
}

// ResolveUsers doi openId (CMS) -> actorId (Chat), tao actor neu chua co.
func (h *Handler) ResolveUsers(c *fiber.Ctx) error {
	var req resolveReq
	if err := c.BodyParser(&req); err != nil {
		return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	people := make([]cms.Person, 0, len(req.Items))
	for _, it := range req.Items {
		people = append(people, cms.Person{OpenID: it.OpenID, CMSUserID: it.CMSUserID, Name: it.Name})
	}

	users, err := h.sync.ResolveUsers(c.UserContext(), people)
	if err != nil {
		return fail(c, err)
	}

	items := make([]fiber.Map, 0, len(users))
	for _, u := range users {
		items = append(items, fiber.Map{
			"openId":   u.OpenID,
			"actorId":  u.ActorID,
			"name":     u.DisplayName,
			"presence": presenceLabel(u.Presence),
		})
	}
	return c.JSON(fiber.Map{"items": items})
}

func presenceLabel(p domain.Presence) string {
	if p == domain.PresenceOnline {
		return "on"
	}
	return "off"
}

// ---------------- messages ----------------

type botInfo struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type postMessageReq struct {
	GroupID      string         `json:"groupId"`
	Text         string         `json:"text"`
	Card         domain.JSONMap `json:"card"`
	CardVersion  int            `json:"cardVersion"`
	ThreadRootID string         `json:"threadRootId"`
	Bot          *botInfo       `json:"bot"`
}

// PostMessage: CMS gui tin / card vao nhom.
func (h *Handler) PostMessage(c *fiber.Ctx) error {
	return h.idempotent(c, "messages.post", func() (int, any, error) {
		var req postMessageReq
		if err := c.BodyParser(&req); err != nil {
			return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
		}
		if req.Text == "" && req.Card == nil {
			return 0, nil, apperr.Invalid("empty_message", "Phai co text hoac card")
		}

		conv, err := h.resolveGroup(c, req.GroupID)
		if err != nil {
			return 0, nil, err
		}
		threadID, err := optionalUUID(req.ThreadRootID)
		if err != nil {
			return 0, nil, err
		}

		msg, err := h.msgs.PostBot(c.UserContext(), message.BotMessageInput{
			ConversationID: conv.ID,
			ThreadID:       threadID,
			Content:        req.Text,
			Card:           req.Card,
			CardVersion:    req.CardVersion,
			BotSender:      botSender(req.Bot),
		})
		if err != nil {
			return 0, nil, err
		}
		h.cards.Broadcast(c.UserContext(), msg, realtime.EventMessageNew)

		return fiber.StatusOK, fiber.Map{
			"messageId":   msg.ID.String(),
			"cardVersion": msg.CardVersion,
		}, nil
	})
}

type patchMessageReq struct {
	Card        domain.JSONMap `json:"card"`
	CardVersion int            `json:"cardVersion"`
}

// PatchMessage: CMS thay card. Chi ap dung khi cardVersion LON HON ban dang luu.
func (h *Handler) PatchMessage(c *fiber.Ctx) error {
	return h.idempotent(c, "messages.patch", func() (int, any, error) {
		id, err := paramUUID(c, "id")
		if err != nil {
			return 0, nil, err
		}
		var req patchMessageReq
		if err := c.BodyParser(&req); err != nil {
			return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
		}
		if req.Card == nil {
			return 0, nil, apperr.Invalid("card_required", "Thieu card")
		}

		current, err := h.store.Messages().GetByID(c.UserContext(), id)
		if err != nil {
			return 0, nil, err
		}

		applied, err := h.cards.ApplyCardPatch(c.UserContext(), id, req.Card, req.CardVersion)
		if err != nil {
			return 0, nil, err
		}
		if !applied {
			return fiber.StatusOK, fiber.Map{
				"ok": true, "ignored": true, "cardVersion": current.CardVersion,
			}, nil
		}
		return fiber.StatusOK, fiber.Map{"ok": true, "cardVersion": req.CardVersion}, nil
	})
}

type threadReq struct {
	Text string   `json:"text"`
	Bot  *botInfo `json:"bot"`
}

// ThreadReply: CMS ghi dong he thong vao thread cua card.
func (h *Handler) ThreadReply(c *fiber.Ctx) error {
	return h.idempotent(c, "messages.thread", func() (int, any, error) {
		rootID, err := paramUUID(c, "id")
		if err != nil {
			return 0, nil, err
		}
		var req threadReq
		if err := c.BodyParser(&req); err != nil {
			return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
		}
		if strings.TrimSpace(req.Text) == "" {
			return 0, nil, apperr.Invalid("text_required", "Thieu text")
		}

		root, err := h.store.Messages().GetByID(c.UserContext(), rootID)
		if err != nil {
			return 0, nil, err
		}
		threadID := root.ID
		if root.ThreadID != nil {
			threadID = *root.ThreadID
		}

		msg, err := h.msgs.PostBot(c.UserContext(), message.BotMessageInput{
			ConversationID: root.ConversationID,
			ThreadID:       &threadID,
			Content:        req.Text,
			BotSender:      botSender(req.Bot),
		})
		if err != nil {
			return 0, nil, err
		}
		h.cards.Broadcast(c.UserContext(), msg, realtime.EventMessageNew)
		return fiber.StatusOK, fiber.Map{"messageId": msg.ID.String()}, nil
	})
}

// ---------------- notifications ----------------

type notificationReq struct {
	ActorID   string `json:"actorId"`
	Text      string `json:"text"`
	MessageID string `json:"messageId"`
}

func (h *Handler) CreateNotification(c *fiber.Ctx) error {
	return h.idempotent(c, "notifications.post", func() (int, any, error) {
		var req notificationReq
		if err := c.BodyParser(&req); err != nil {
			return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
		}
		if req.ActorID == "" || strings.TrimSpace(req.Text) == "" {
			return 0, nil, apperr.Invalid("invalid_request", "Thieu actorId hoac text")
		}

		user, err := h.store.Users().GetByActorID(c.UserContext(), req.ActorID)
		if err != nil {
			return 0, nil, err
		}
		messageID, err := optionalUUID(req.MessageID)
		if err != nil {
			return 0, nil, err
		}

		n, err := h.notifs.Create(c.UserContext(), notification.CreateInput{
			UserID: user.ID, Text: req.Text, MessageID: messageID,
		})
		if err != nil {
			return 0, nil, err
		}
		return fiber.StatusOK, fiber.Map{
			"id":        n.ID.String(),
			"actorId":   user.ActorID,
			"text":      n.Text,
			"messageId": req.MessageID,
			"createdAt": n.CreatedAt.UnixMilli(),
			"read":      n.Read,
		}, nil
	})
}

// ---------------- groups ----------------

type putGroupReq struct {
	Name    string `json:"name"`
	Members []struct {
		OpenID    string   `json:"openId"`
		CMSUserID int64    `json:"cmsUserId"`
		Name      string   `json:"name"`
		AvatarURL string   `json:"avatarUrl"`
		Sources   []string `json:"sources"`
		TeamCodes []string `json:"teamCodes"`
	} `json:"members"`
}

// PutGroup: CMS day nhom + danh sach thanh vien (thay the toan bo).
func (h *Handler) PutGroup(c *fiber.Ctx) error {
	return h.idempotent(c, "groups.put", func() (int, any, error) {
		chatID := c.Params("id")
		if chatID == "" {
			return 0, nil, apperr.Invalid("invalid_request", "Thieu chatId")
		}
		var req putGroupReq
		if err := c.BodyParser(&req); err != nil {
			return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
		}

		group := cms.Group{ChatID: chatID, Name: req.Name}
		for _, m := range req.Members {
			group.Members = append(group.Members, cms.Member{
				Person: cms.Person{
					CMSUserID: m.CMSUserID, OpenID: m.OpenID,
					Name: m.Name, AvatarURL: m.AvatarURL,
				},
				Sources:   m.Sources,
				TeamCodes: m.TeamCodes,
			})
		}

		conv, err := h.sync.ApplyGroup(c.UserContext(), group)
		if err != nil {
			return 0, nil, err
		}
		return fiber.StatusOK, fiber.Map{
			"id":          chatID,
			"memberCount": len(conv.Members),
		}, nil
	})
}

func (h *Handler) GroupMembers(c *fiber.Ctx) error {
	conv, err := h.resolveGroup(c, c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	members, err := h.store.Conversations().ListMembers(c.UserContext(), conv.ID)
	if err != nil {
		return fail(c, err)
	}

	items := make([]fiber.Map, 0, len(members))
	for _, m := range members {
		item := fiber.Map{"id": "", "name": "", "presence": "off", "avatarUrl": "", "role": string(m.Role)}
		if m.User != nil {
			item["id"] = m.User.ActorID
			item["name"] = m.User.DisplayName
			item["avatarUrl"] = m.User.AvatarURL
			item["presence"] = presenceLabel(m.User.Presence)
			item["title"] = m.User.DisplayRole()
		}
		items = append(items, item)
	}
	return c.JSON(fiber.Map{"items": items})
}

// ---------------- helper ----------------

// resolveGroup nhan ca chatId cua CMS (wg_...) lan UUID noi bo.
func (h *Handler) resolveGroup(c *fiber.Ctx, id string) (*domain.Conversation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperr.Invalid("invalid_request", "Thieu groupId")
	}
	if parsed, err := uuid.Parse(id); err == nil {
		return h.store.Conversations().GetByID(c.UserContext(), parsed)
	}
	return h.store.Conversations().GetByExternalID(c.UserContext(), id)
}

func botSender(b *botInfo) domain.JSONMap {
	if b == nil {
		return nil
	}
	return domain.JSONMap{"id": b.ID, "name": b.Name}
}

func paramUUID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid_id", name+" khong hop le")
	}
	return id, nil
}

func optionalUUID(s string) (*uuid.UUID, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, apperr.Invalid("invalid_id", "id khong hop le: "+s)
	}
	return &id, nil
}

// idempotent: gap lai Idempotency-Key thi tra nguyen ket qua cu, khong lam lai.
func (h *Handler) idempotent(c *fiber.Ctx, endpoint string, fn func() (int, any, error)) error {
	key := strings.TrimSpace(c.Get(HeaderIdempotent))
	ctx := c.UserContext()

	if key != "" {
		if rec, err := h.store.Idempotency().Get(ctx, key); err == nil && rec != nil {
			return c.Status(rec.StatusCode).JSON(rec.Response)
		}
	}

	status, body, err := fn()
	if err != nil {
		return fail(c, err)
	}
	if status == 0 {
		status = fiber.StatusOK
	}

	if key != "" {
		_ = h.store.Idempotency().Save(ctx, &domain.IdempotencyRecord{
			Key: key, Endpoint: endpoint, StatusCode: status,
			Response: toJSONMap(body), CreatedAt: time.Now().UTC(),
		})
	}
	return c.Status(status).JSON(body)
}
