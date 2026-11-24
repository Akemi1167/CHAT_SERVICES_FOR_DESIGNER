package clientapi

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/card"
	"github.com/abc-workspace/chat-service/internal/service/cmssync"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
	"github.com/abc-workspace/chat-service/internal/service/notification"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/clientview"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

type Handler struct {
	store  repository.Store
	convs  *conversation.Service
	msgs   *message.Service
	cards  *card.Service
	notifs *notification.Service
	sync   *cmssync.Service
}

func NewHandler(
	store repository.Store,
	convs *conversation.Service,
	msgs *message.Service,
	cards *card.Service,
	notifs *notification.Service,
	sync *cmssync.Service,
) *Handler {
	return &Handler{store: store, convs: convs, msgs: msgs, cards: cards, notifs: notifs, sync: sync}
}

// Register gan route; router da di qua middleware auth.
func (h *Handler) Register(router fiber.Router) {
	router.Get("/auth/me", h.Me)
	router.Get("/bootstrap", h.Bootstrap)
	router.Post("/groups/refresh", h.RefreshGroups)
	router.Get("/groups/:id/messages", h.ListMessages)
	router.Post("/groups/:id/messages", h.SendMessage)
	router.Post("/groups/:id/read", h.MarkRead)
	router.Get("/messages/:id/replies", h.Replies)
	router.Get("/cards/:code", h.CardThread)
	router.Post("/messages/:id/actions", h.RunAction)
	router.Post("/forms/:formId/submit", h.SubmitForm)
	router.Get("/notifications", h.ListNotifications)
	router.Post("/notifications/read", h.ReadNotifications)
}

// ---------------- me / bootstrap ----------------

func (h *Handler) Me(c *fiber.Ctx) error {
	u, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, fiber.Map{"me": userView(u)})
}

// Bootstrap: tat ca thu FE can khi mo app — chi gom nhom user la thanh vien,
// card da loc nut theo nguoi xem.
//
// Thanh ben kieu Lark (chats = nhom + thread tron chung) phan trang de FE lazy load:
//
//	GET /bootstrap                        me + groups + chats (trang dau) + notifications
//	GET /bootstrap?chatCursor=<cursor>    CHI trang chats tiep theo (khong tai lai nhom/thong bao)
//	chatLimit=   so dong moi trang (mac dinh 20, toi da 100)
//
// groups van tra du (toi da 100) — dung tra thanh vien / to (cms.teams), khong phai de ve thanh ben.
func (h *Handler) Bootstrap(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	ctx := c.UserContext()

	chatCursor := strings.TrimSpace(c.Query("chatCursor"))
	chats, chatsPage, err := h.chatsPage(c, me,
		paging.Normalize(chatCursor, int32(c.QueryInt("chatLimit", defaultChatLimit))))
	if err != nil {
		return fail(c, err)
	}
	if chatCursor != "" {
		return ok(c, fiber.Map{"chats": chats, "chatsPageInfo": chatsPage})
	}

	convs, _, err := h.convs.List(ctx, me.ID, repository.ConversationFilter{}, paging.Normalize("", 100))
	if err != nil {
		return fail(c, err)
	}

	groups := make([]fiber.Map, 0, len(convs))
	for i := range convs {
		groups = append(groups, h.groupView(c, &convs[i], me))
	}

	notifs, _, err := h.notifs.List(ctx, me.ID, repository.NotificationFilter{}, paging.Normalize("", 30))
	if err != nil {
		return fail(c, err)
	}
	unread, err := h.notifs.CountUnread(ctx, me.ID)
	if err != nil {
		return fail(c, err)
	}

	return ok(c, fiber.Map{
		"me":                  userView(me),
		"groups":              groups,
		"chats":               chats,
		"chatsPageInfo":       chatsPage,
		"notifications":       clientview.Notifications(notifs),
		"unreadNotifications": unread,
	})
}

// defaultChatLimit: so dong thanh ben trang dau trong bootstrap (vua mot man hinh).
const defaultChatLimit = 20

// RefreshGroups: FE goi sau khi vao nhom bang link moi -> Chat hoi lai CMS ngay.
func (h *Handler) RefreshGroups(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	if h.sync == nil || !h.sync.Enabled() {
		return fail(c, apperr.Invalid("cms_disabled",
			"Chua bat tich hop CMS (CMS_ENABLED=false) nen khong co gi de dong bo"))
	}
	n, err := h.sync.SyncAll(c.UserContext())
	if err != nil {
		return fail(c, apperr.Internal("cms_unreachable", "Khong hoi duoc CMS, thu lai sau").Wrap(err))
	}
	convs, _, err := h.convs.List(c.UserContext(), me.ID, repository.ConversationFilter{}, paging.Normalize("", 100))
	if err != nil {
		return fail(c, err)
	}
	groups := make([]fiber.Map, 0, len(convs))
	for i := range convs {
		groups = append(groups, h.groupView(c, &convs[i], me))
	}
	return ok(c, fiber.Map{"synced": n, "groups": groups})
}

// ---------------- tin nhan ----------------

func (h *Handler) ListMessages(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	conv, err := h.resolveGroup(c)
	if err != nil {
		return fail(c, err)
	}
	threadID, err := optionalUUID(c.Query("threadRootId"))
	if err != nil {
		return fail(c, err)
	}

	// Lich su nhom mac dinh CHI co tin goc: tin tra loi trong thread gom vao tin
	// goc (threadCount), bam vao moi tai qua ?threadRootId= hoac /messages/:id/replies?mode=thread.
	// includeThread=true: lay ca tin trong thread (hanh vi cu, de debug / FE cu).
	onlyRoot := threadID == nil && !c.QueryBool("includeThread", false)

	items, info, err := h.msgs.List(c.UserContext(), message.ListInput{
		ActorID: me.ID, ConversationID: conv.ID, ThreadID: threadID, OnlyRoot: onlyRoot,
	}, paging.Normalize(c.Query("cursor"), int32(c.QueryInt("limit", 0))))
	if err != nil {
		return fail(c, err)
	}

	return ok(c, fiber.Map{
		"messages": h.messageViews(items, me.ActorID),
		"pageInfo": pageView(info),
	})
}

type sendMessageReq struct {
	Text string `json:"text"`
	// threadRootId = tin mo thread (card/yeu cau); replyToId = tin duoc trich dan.
	ThreadRootID string              `json:"threadRootId"`
	ReplyToID    string              `json:"replyToId"`
	Attachments  []attachmentPayload `json:"attachments"`
	Metadata     domain.JSONMap      `json:"metadata"`
	ClientMsgID  string              `json:"clientMsgId"`
}

type attachmentPayload struct {
	Kind       string `json:"kind"`
	URL        string `json:"url"`
	Name       string `json:"name"`
	MimeType   string `json:"mimeType"`
	SizeBytes  int64  `json:"sizeBytes"`
	Width      int32  `json:"width"`
	Height     int32  `json:"height"`
	DurationMs int32  `json:"durationMs"`
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

func (h *Handler) SendMessage(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	conv, err := h.resolveGroup(c)
	if err != nil {
		return fail(c, err)
	}
	var req sendMessageReq
	if err := c.BodyParser(&req); err != nil {
		return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	threadID, err := optionalUUID(req.ThreadRootID)
	if err != nil {
		return fail(c, err)
	}
	replyTo, err := optionalUUID(req.ReplyToID)
	if err != nil {
		return fail(c, err)
	}

	msgType := domain.MessageText
	if len(req.Attachments) > 0 {
		msgType = domain.MessageFile
	}
	res, err := h.msgs.Send(c.UserContext(), message.SendInput{
		ActorID:        me.ID,
		ConversationID: conv.ID,
		Type:           msgType,
		Content:        req.Text,
		ThreadID:       threadID,
		ReplyToID:      replyTo,
		Attachments:    toDomainAttachments(req.Attachments),
		Metadata:       req.Metadata,
		ClientMsgID:    req.ClientMsgID,
	})
	if err != nil {
		return fail(c, err)
	}
	return ok(c, fiber.Map{
		"message":    h.messageView(res.Message, me.ActorID),
		"duplicated": res.Duplicated,
	})
}

type markReadReq struct {
	MessageID string `json:"messageId"`
}

func (h *Handler) MarkRead(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	conv, err := h.resolveGroup(c)
	if err != nil {
		return fail(c, err)
	}
	var req markReadReq
	if err := c.BodyParser(&req); err != nil {
		return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	msgID, err := uuid.Parse(req.MessageID)
	if err != nil {
		return fail(c, apperr.Invalid("invalid_id", "messageId khong hop le"))
	}

	unread, err := h.msgs.MarkRead(c.UserContext(), me.ID, conv.ID, msgID)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, fiber.Map{"unreadCount": unread})
}

// Replies: phan hoi cua 1 tin nhan.
//
//	?mode=direct (mac dinh) — tin tra loi truc tiep tin nay
//	?mode=thread            — ca thread do tin nay mo (dung cho card/yeu cau)
func (h *Handler) Replies(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	msgID, err := paramUUID(c, "id")
	if err != nil {
		return fail(c, err)
	}

	res, err := h.msgs.Replies(c.UserContext(), message.RepliesInput{
		ActorID:   me.ID,
		MessageID: msgID,
		Mode:      repository.ReplyMode(c.Query("mode")),
	}, paging.Normalize(c.Query("cursor"), int32(c.QueryInt("limit", 0))))
	if err != nil {
		return fail(c, err)
	}

	return ok(c, fiber.Map{
		"root":        h.messageView(res.Root, me.ActorID),
		"replies":     h.messageViews(res.Replies, me.ActorID),
		"replyCount":  res.DirectCount,
		"threadCount": res.ThreadCount,
		"pageInfo":    pageView(res.PageInfo),
	})
}

// chatsPage: 1 trang thanh ben kieu Lark — nhom va thread tron chung, moi hoat dong nhat truoc.
//
//	type=group  : bam vao -> GET /groups/:id/messages
//	type=thread : bam vao -> GET /groups/:group.id/messages?threadRootId=<id>
func (h *Handler) chatsPage(c *fiber.Ctx, me *domain.User, p paging.Page) ([]fiber.Map, fiber.Map, error) {
	items, info, err := h.msgs.ListSidebar(c.UserContext(), me.ID, p)
	if err != nil {
		return nil, nil, err
	}
	chats := make([]fiber.Map, 0, len(items))
	for _, it := range items {
		group := fiber.Map{}
		if it.Conversation != nil {
			chatID := ""
			if it.Conversation.ExternalID != nil {
				chatID = *it.Conversation.ExternalID
			}
			group = fiber.Map{"id": it.Conversation.ID.String(), "chatId": chatID,
				"name": it.Conversation.Name, "avatarUrl": it.Conversation.AvatarURL}
		}
		item := fiber.Map{
			"type":           string(it.Kind),
			"group":          group,
			"lastActivityAt": it.LastActivityAt,
			"preview":        clientview.Preview(it.LastMessage),
			"lastMessage":    nil,
		}
		if it.LastMessage != nil {
			item["lastMessage"] = h.messageView(it.LastMessage, me.ActorID)
		}
		if it.Kind == repository.SidebarThread {
			item["id"] = it.Root.ID.String() // = threadRootId
			item["title"] = clientview.ThreadTitle(it.Root)
			item["root"] = h.messageView(it.Root, me.ActorID)
			item["threadCount"] = it.ThreadCount
			item["replyCount"] = it.UserReplyCount
		} else {
			item["id"] = it.Conversation.ID.String()
			item["title"] = it.Conversation.Name
			item["unreadCount"] = it.UnreadCount
		}
		chats = append(chats, item)
	}
	return chats, pageView(info), nil
}

// CardThread: panel chi tiet 1 don theo ma (card.header.code), vd WEB-20261123-0001.
//
//	card       tin chua card (da loc visibleTo, kem actions nguoi xem duoc bam)
//	root       card goc cua thread (khac card khi day la card con -P1/-E1...)
//	links      cac khoi {"type":"links"} cua card (vd "Google Drive"), rong = chua co
//	activity   dong dien bien bot/he thong trong thread (cu -> moi)
//	discussion tin nguoi dung trao doi trong thread (cu -> moi)
//
// Phan trang thread bang ?cursor=&limit= nhu /messages/:id/replies?mode=thread.
func (h *Handler) CardThread(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	res, err := h.cards.ThreadByCode(c.UserContext(), me.ID, c.Params("code"),
		paging.Normalize(c.Query("cursor"), int32(c.QueryInt("limit", 0))))
	if err != nil {
		return fail(c, err)
	}
	return ok(c, fiber.Map{
		"card":        h.messageView(res.Card, res.ViewerActorID),
		"root":        h.messageView(res.Root, res.ViewerActorID),
		"links":       res.Links,
		"activity":    h.messageViews(res.Activity, res.ViewerActorID),
		"discussion":  h.messageViews(res.Discussion, res.ViewerActorID),
		"threadCount": res.ThreadCount,
		"pageInfo":    pageView(res.PageInfo),
	})
}

// ---------------- card / form (chuyen sang CMS) ----------------

type actionReq struct {
	Action string         `json:"action"`
	Values domain.JSONMap `json:"values"`
}

func (h *Handler) RunAction(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	msgID, err := paramUUID(c, "id")
	if err != nil {
		return fail(c, err)
	}
	var req actionReq
	if err := c.BodyParser(&req); err != nil {
		return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}

	res, err := h.cards.RunAction(c.UserContext(), card.ActionInput{
		ActorID: me.ID, MessageID: msgID, Action: req.Action, Values: req.Values,
	})
	if err != nil {
		return fail(c, err)
	}
	return ok(c, res)
}

type submitFormReq struct {
	Values  domain.JSONMap `json:"values"`
	Context struct {
		GroupID      string `json:"groupId"`
		ThreadRootID string `json:"threadRootId"`
	} `json:"context"`
}

// SubmitForm: FE gui form (vd create_task voi team_id) -> Chat ky va chuyen sang CMS.
func (h *Handler) SubmitForm(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	var req submitFormReq
	if err := c.BodyParser(&req); err != nil {
		return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	conv, err := h.lookupGroup(c, req.Context.GroupID)
	if err != nil {
		return fail(c, err)
	}
	threadID, err := optionalUUID(req.Context.ThreadRootID)
	if err != nil {
		return fail(c, err)
	}

	res, err := h.cards.SubmitForm(c.UserContext(), card.SubmitFormInput{
		ActorID:        me.ID,
		ConversationID: conv.ID,
		ThreadID:       threadID,
		FormID:         c.Params("formId"),
		Values:         req.Values,
	})
	if err != nil {
		return fail(c, err)
	}
	return ok(c, res)
}

// ---------------- thong bao ----------------

func (h *Handler) ListNotifications(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	items, info, err := h.notifs.List(c.UserContext(), me.ID,
		repository.NotificationFilter{UnreadOnly: c.QueryBool("unread", false)},
		paging.Normalize(c.Query("cursor"), int32(c.QueryInt("limit", 0))))
	if err != nil {
		return fail(c, err)
	}
	unread, err := h.notifs.CountUnread(c.UserContext(), me.ID)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, fiber.Map{"notifications": clientview.Notifications(items), "unread": unread, "pageInfo": pageView(info)})
}

type readNotificationsReq struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

func (h *Handler) ReadNotifications(c *fiber.Ctx) error {
	me, err := h.currentUser(c)
	if err != nil {
		return fail(c, err)
	}
	var req readNotificationsReq
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
		}
	}
	ids := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return fail(c, apperr.Invalid("invalid_id", "id thong bao khong hop le: "+raw))
		}
		ids = append(ids, id)
	}

	n, err := h.notifs.MarkRead(c.UserContext(), me.ID, ids, req.All || len(ids) == 0)
	if err != nil {
		return fail(c, err)
	}
	unread, err := h.notifs.CountUnread(c.UserContext(), me.ID)
	if err != nil {
		return fail(c, err)
	}
	return ok(c, fiber.Map{"updated": n, "unread": unread})
}

// ---------------- view helper ----------------

func userView(u *domain.User) fiber.Map {
	return clientview.User(u)
}

func (h *Handler) groupView(c *fiber.Ctx, conv *domain.Conversation, me *domain.User) fiber.Map {
	members := make([]fiber.Map, 0, len(conv.Members))
	for _, m := range conv.Members {
		item := fiber.Map{
			"userId":    m.UserID.String(),
			"role":      string(m.Role),
			"sources":   []string(m.Sources),
			"teamCodes": []string(m.TeamCodes),
		}
		if m.User != nil {
			item["actorId"] = m.User.ActorID
			item["name"] = m.User.DisplayName
			item["avatarUrl"] = m.User.AvatarURL
			item["presence"] = string(m.User.LivePresence())
			item["title"] = m.User.DisplayRole()
		}
		members = append(members, item)
	}

	chatID := ""
	if conv.ExternalID != nil {
		chatID = *conv.ExternalID
	}
	return fiber.Map{
		"id":          conv.ID.String(),
		"chatId":      chatID,
		"name":        conv.Name,
		"source":      conv.Source,
		"unreadCount": conv.UnreadCount,
		"cms":         map[string]any(conv.CMSInfo),
		"members":     members,
	}
}

// pageView doi pageInfo sang camelCase cho be mat /api/client/v1 (hop dong voi CMS/FE).
func pageView(info paging.PageInfo) fiber.Map {
	return clientview.Page(info.NextCursor, info.HasMore)
}

func (h *Handler) messageViews(items []domain.Message, actorID string) []fiber.Map {
	out := make([]fiber.Map, 0, len(items))
	for i := range items {
		out = append(out, clientview.Message(&items[i], actorID))
	}
	return out
}

// messageView loc card theo nguoi xem truoc khi tra cho UI.
// Hinh dang do clientview dung, de REST va WebSocket khong lech ten truong.
func (h *Handler) messageView(m *domain.Message, actorID string) fiber.Map {
	return clientview.Message(m, actorID)
}

func (h *Handler) currentUser(c *fiber.Ctx) (*domain.User, error) {
	id, err := middleware.Identity(c)
	if err != nil {
		return nil, err
	}
	return h.store.Users().GetByID(c.UserContext(), id.UserID)
}

// resolveGroup nhan ca chatId cua CMS (wg_...) lan UUID noi bo.
func (h *Handler) resolveGroup(c *fiber.Ctx) (*domain.Conversation, error) {
	return h.lookupGroup(c, c.Params("id"))
}

func (h *Handler) lookupGroup(c *fiber.Ctx, id string) (*domain.Conversation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apperr.Invalid("invalid_request", "Thieu groupId")
	}
	if parsed, err := uuid.Parse(id); err == nil {
		return h.store.Conversations().GetByID(c.UserContext(), parsed)
	}
	return h.store.Conversations().GetByExternalID(c.UserContext(), id)
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
