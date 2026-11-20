package botapi

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/card"
	"github.com/abc-workspace/chat-service/internal/service/cmssync"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/service/notification"
	"github.com/abc-workspace/chat-service/internal/service/presence"
)

type Handler struct {
	store    repository.Store
	sync     *cmssync.Service
	msgs     *message.Service
	cards    *card.Service
	notifs   *notification.Service
	presence *presence.Service
}

func NewHandler(
	store repository.Store,
	sync *cmssync.Service,
	msgs *message.Service,
	cards *card.Service,
	notifs *notification.Service,
	presenceSvc *presence.Service,
) *Handler {
	return &Handler{store: store, sync: sync, msgs: msgs, cards: cards, notifs: notifs, presence: presenceSvc}
}

// Register gan toan bo route Bot API.
func (h *Handler) Register(router fiber.Router) {
	router.Post("/users/resolve", h.ResolveUsers)
	router.Post("/messages", h.PostMessage)
	router.Patch("/messages/:id", h.PatchMessage)
	router.Post("/messages/:id/thread", h.ThreadReply)
	router.Post("/messages/:id/pin", h.PinMessage)
	router.Put("/presence", h.SetPresence)
	router.Get("/attachments/:id", h.GetAttachment)
	router.Post("/notifications", h.CreateNotification)
	router.Put("/groups/:id", h.PutGroup)
	router.Get("/groups/:id/members", h.GroupMembers)
	router.Get("/groups", h.ListGroups)
	router.Get("/groups/:id", h.GetGroup)
	router.Get("/users/:id", h.GetUser)
	router.Post("/users/batch", h.BatchUsers)
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
			"presence": presenceLabel(u.LivePresence()),
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

	// Hai phan TUY CHON, gop vao de CMS khong phai goi 3 request cho mot lan
	// doi trang thai don. Khong gui thi hanh vi y nhu truoc.
	Thread *threadReq        `json:"thread"`
	Notify []notificationReq `json:"notify"`
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
			// cardVersion khong lon hon ban dang luu => CMS dang gui trang thai CU.
			// Bo qua CA thread va notify: ghi them dong dien bien cu vao nhom la sai.
			return fiber.StatusOK, fiber.Map{
				"ok": true, "ignored": true, "cardVersion": current.CardVersion,
			}, nil
		}

		out := fiber.Map{"ok": true, "cardVersion": req.CardVersion}

		// Card la viec chinh va da xong. Hai viec phu loi thi KHONG tra 5xx:
		// CMS se retry, nhung cardVersion khong tang nua nen lan retry chi nhan
		// `ignored: true` — mat luon ca thread/notify. Bao loi theo tung phan.
		if req.Thread != nil {
			if res, terr := h.addThreadLine(c, current, *req.Thread); terr != nil {
				out["thread"] = partError(terr)
			} else {
				out["thread"] = res
			}
		}
		if len(req.Notify) > 0 {
			items := make([]fiber.Map, 0, len(req.Notify))
			for _, n := range req.Notify {
				if n.MessageID == "" {
					n.MessageID = current.ID.String() // mac dinh tro ve chinh card nay
				}
				if res, nerr := h.addNotification(c, n); nerr != nil {
					items = append(items, partError(nerr))
				} else {
					items = append(items, res)
				}
			}
			out["notify"] = items
		}
		return fiber.StatusOK, out, nil
	})
}

// partError: loi cua mot phan phu trong PATCH gop, cung hinh dang voi loi chung.
func partError(err error) fiber.Map {
	e := apperr.From(err)
	return fiber.Map{"error": fiber.Map{"code": e.Code, "message": e.Message}}
}

// addThreadLine ghi mot dong he thong vao thread cua card. Dung chung boi
// POST /messages/:id/thread va PATCH /messages/:id (field `thread`).
func (h *Handler) addThreadLine(c *fiber.Ctx, root *domain.Message, req threadReq) (fiber.Map, error) {
	if strings.TrimSpace(req.Text) == "" {
		return nil, apperr.Invalid("text_required", "Thieu text")
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
		return nil, err
	}
	h.cards.Broadcast(c.UserContext(), msg, realtime.EventMessageNew)
	return fiber.Map{"messageId": msg.ID.String()}, nil
}

// addNotification tao 1 thong bao. Dung chung boi POST /notifications va
// PATCH /messages/:id (field `notify`).
func (h *Handler) addNotification(c *fiber.Ctx, req notificationReq) (fiber.Map, error) {
	if req.ActorID == "" || strings.TrimSpace(req.Text) == "" {
		return nil, apperr.Invalid("invalid_request", "Thieu actorId hoac text")
	}
	user, err := h.store.Users().GetByActorID(c.UserContext(), req.ActorID)
	if err != nil {
		return nil, err
	}
	messageID, err := optionalUUID(req.MessageID)
	if err != nil {
		return nil, err
	}
	n, err := h.notifs.Create(c.UserContext(), notification.CreateInput{
		UserID: user.ID, Text: req.Text, MessageID: messageID,
	})
	if err != nil {
		return nil, err
	}
	return fiber.Map{
		"id":        n.ID.String(),
		"actorId":   user.ActorID,
		"text":      n.Text,
		"messageId": req.MessageID,
		"createdAt": n.CreatedAt.UnixMilli(),
		"read":      n.Read,
	}, nil
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
		res, err := h.addThreadLine(c, root, req)
		if err != nil {
			return 0, nil, err
		}
		return fiber.StatusOK, res, nil
	})
}

// ---------------- ghim tin nhan ----------------

type pinReq struct {
	// Mac dinh true. Gui {"pinned": false} de bo ghim.
	Pinned *bool `json:"pinned"`
}

// PinMessage: CMS ghim / bo ghim mot tin nhan trong nhom.
func (h *Handler) PinMessage(c *fiber.Ctx) error {
	return h.idempotent(c, "messages.pin", func() (int, any, error) {
		id, err := paramUUID(c, "id")
		if err != nil {
			return 0, nil, err
		}
		pinned := true
		if len(c.Body()) > 0 {
			var req pinReq
			if err := c.BodyParser(&req); err != nil {
				return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
			}
			if req.Pinned != nil {
				pinned = *req.Pinned
			}
		}

		// CMS khong phai thanh vien nhom nen khong truyen actor.
		msg, err := h.msgs.SetPinned(c.UserContext(), nil, id, pinned)
		if err != nil {
			return 0, nil, err
		}
		return fiber.StatusOK, fiber.Map{
			"ok": true, "messageId": msg.ID.String(), "pinned": msg.Pinned,
		}, nil
	})
}

// ---------------- presence ----------------

type presenceReq struct {
	Items []struct {
		ActorID string `json:"actorId"`
		Status  string `json:"status"` // on | off | off_duty
		Reason  string `json:"reason"`
	} `json:"items"`
}

// SetPresence: CMS bao trang thai nhan su (vd lich OFF) cho nhieu nguoi cung luc.
func (h *Handler) SetPresence(c *fiber.Ctx) error {
	return h.idempotent(c, "presence.put", func() (int, any, error) {
		var req presenceReq
		if err := c.BodyParser(&req); err != nil {
			return 0, nil, apperr.Invalid("invalid_body", "Body khong hop le")
		}
		items := make([]presence.Item, 0, len(req.Items))
		for _, it := range req.Items {
			items = append(items, presence.Item{ActorID: it.ActorID, Status: it.Status, Reason: it.Reason})
		}

		updated, err := h.presence.SetMany(c.UserContext(), items)
		if err != nil {
			return 0, nil, err
		}
		return fiber.StatusOK, fiber.Map{"updated": updated}, nil
	})
}

// ---------------- tep dinh kem ----------------

// GetAttachment tra metadata cua tep. Chat KHONG luu file, chi luu URL do
// service upload/CDN cung cap; them ?redirect=true de chuyen thang toi URL do.
func (h *Handler) GetAttachment(c *fiber.Ctx) error {
	id, err := paramUUID(c, "id")
	if err != nil {
		return fail(c, err)
	}
	att, msg, err := h.msgs.Attachment(c.UserContext(), nil, id)
	if err != nil {
		return fail(c, err)
	}
	if c.QueryBool("redirect", false) && att.URL != "" {
		return c.Redirect(att.URL, fiber.StatusFound)
	}
	return c.JSON(fiber.Map{
		"id": att.ID.String(), "kind": att.Kind, "url": att.URL, "name": att.Name,
		"mimeType": att.MimeType, "sizeBytes": att.SizeBytes,
		"width": att.Width, "height": att.Height, "durationMs": att.DurationMs,
		"messageId": msg.ID.String(), "groupId": groupIDOf(msg),
		"createdAt": att.CreatedAt,
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
		res, err := h.addNotification(c, req)
		if err != nil {
			return 0, nil, err
		}
		return fiber.StatusOK, res, nil
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
			item["presence"] = presenceLabel(m.User.LivePresence())
			item["title"] = m.User.DisplayRole()
		}
		items = append(items, item)
	}
	return c.JSON(fiber.Map{"items": items})
}

// ---------------- tra cuu (spec 7.7: CMS chua dung, de san) ----------------

// ListGroups liet ke nhom Chat dang giu (chu yeu de doi chieu khi debug dong bo).
func (h *Handler) ListGroups(c *fiber.Ctx) error {
	// ?q= tim theo ten, ?source= loc cms|local (rong = tat ca).
	convs, err := h.store.Conversations().ListAll(c.UserContext(), repository.ConversationFilter{
		Query:  c.Query("q"),
		Source: c.Query("source"),
	}, paging.Normalize("", 200))
	if err != nil {
		return fail(c, err)
	}
	items := make([]fiber.Map, 0, len(convs))
	for i := range convs {
		items = append(items, groupView(&convs[i]))
	}
	return c.JSON(fiber.Map{"items": items})
}

func (h *Handler) GetGroup(c *fiber.Ctx) error {
	conv, err := h.resolveGroup(c, c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(groupView(conv))
}

func (h *Handler) GetUser(c *fiber.Ctx) error {
	u, err := h.lookupUser(c, c.Params("id"))
	if err != nil {
		return fail(c, err)
	}
	return c.JSON(userView(u))
}

type batchUsersReq struct {
	// Nhan bat ky loai dinh danh nao: actorId, openId hoac cmsUserId.
	ActorIDs   []string `json:"actorIds"`
	OpenIDs    []string `json:"openIds"`
	CMSUserIDs []int64  `json:"cmsUserIds"`
}

func (h *Handler) BatchUsers(c *fiber.Ctx) error {
	var req batchUsersReq
	if err := c.BodyParser(&req); err != nil {
		return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	ctx := c.UserContext()

	seen := map[uuid.UUID]bool{}
	items := make([]fiber.Map, 0)
	add := func(users []domain.User) {
		for i := range users {
			if seen[users[i].ID] {
				continue
			}
			seen[users[i].ID] = true
			items = append(items, userView(&users[i]))
		}
	}

	if len(req.ActorIDs) > 0 {
		users, err := h.store.Users().ListByActorIDs(ctx, req.ActorIDs)
		if err != nil {
			return fail(c, err)
		}
		add(users)
	}
	if len(req.OpenIDs) > 0 {
		users, err := h.store.Users().ListByOpenIDs(ctx, req.OpenIDs)
		if err != nil {
			return fail(c, err)
		}
		add(users)
	}
	if len(req.CMSUserIDs) > 0 {
		users, err := h.store.Users().ListByCMSUserIDs(ctx, req.CMSUserIDs)
		if err != nil {
			return fail(c, err)
		}
		add(users)
	}
	return c.JSON(fiber.Map{"items": items})
}

// lookupUser nhan actorId (u_68), openId hoac UUID noi bo.
func (h *Handler) lookupUser(c *fiber.Ctx, id string) (*domain.User, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperr.Invalid("invalid_request", "Thieu id")
	}
	ctx := c.UserContext()
	if parsed, err := uuid.Parse(id); err == nil {
		return h.store.Users().GetByID(ctx, parsed)
	}
	if u, err := h.store.Users().GetByActorID(ctx, id); err == nil {
		return u, nil
	}
	return h.store.Users().GetByOpenID(ctx, id)
}

func groupView(c *domain.Conversation) fiber.Map {
	id := c.ID.String()
	if c.ExternalID != nil && *c.ExternalID != "" {
		id = *c.ExternalID
	}
	return fiber.Map{
		"id": id, "internalId": c.ID.String(), "name": c.Name,
		"source": c.Source, "memberCount": len(c.Members),
		"cms": map[string]any(c.CMSInfo),
	}
}

func userView(u *domain.User) fiber.Map {
	view := fiber.Map{
		"id": u.ActorID, "internalId": u.ID.String(), "name": u.DisplayName,
		"openId": u.OpenID, "avatarUrl": u.AvatarURL,
		"presence": presenceLabel(u.LivePresence()), "role": u.DisplayRole(),
		"title": u.DisplayRole(),
	}
	if u.PresenceReason != "" {
		view["presenceReason"] = u.PresenceReason
	}
	if u.CMSUserID != nil {
		view["cmsUserId"] = *u.CMSUserID
	}
	return view
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

// groupIDOf tra chatId cua CMS neu co, nguoc lai tra id noi bo.
func groupIDOf(m *domain.Message) string {
	return m.ConversationID.String()
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
