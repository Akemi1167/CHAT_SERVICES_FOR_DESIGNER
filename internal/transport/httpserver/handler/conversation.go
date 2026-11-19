// Package handler chua REST handler (Fiber). Chi parse/validate input va map output.
package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

type ConversationHandler struct {
	convs *conversation.Service
	store repository.Store
}

func NewConversationHandler(convs *conversation.Service, store repository.Store) *ConversationHandler {
	return &ConversationHandler{convs: convs, store: store}
}

type createConversationReq struct {
	Type          string   `json:"type"` // direct | group
	Name          string   `json:"name"`
	AvatarURL     string   `json:"avatar_url"`
	MemberUserIDs []string `json:"member_user_ids"`
}

func (h *ConversationHandler) Create(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req createConversationReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	members, err := parseUUIDList(req.MemberUserIDs, "member_user_ids")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	conv, err := h.convs.Create(c.UserContext(), conversation.CreateInput{
		ActorID:       id.UserID,
		Type:          domain.ConversationType(req.Type),
		Name:          req.Name,
		AvatarURL:     req.AvatarURL,
		MemberUserIDs: members,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.Created(c, conv)
}

func (h *ConversationHandler) List(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	f, err := conversationFilterFromQuery(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	p := pageFromQuery(c)
	items, info, err := h.convs.List(c.UserContext(), id.UserID, f, p)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, items, info)
}

func (h *ConversationHandler) Get(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := convIDParam(c, h.store)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	conv, err := h.convs.Get(c.UserContext(), id.UserID, convID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, conv)
}

type addMembersReq struct {
	UserIDs []string `json:"user_ids"`
	Role    string   `json:"role"`
}

func (h *ConversationHandler) AddMembers(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := convIDParam(c, h.store)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req addMembersReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	userIDs, err := parseUUIDList(req.UserIDs, "user_ids")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	conv, err := h.convs.AddMembers(c.UserContext(), conversation.AddMembersInput{
		ActorID: id.UserID, ConvID: convID, UserIDs: userIDs,
		Role: domain.MemberRole(req.Role),
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, conv)
}

func (h *ConversationHandler) RemoveMember(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := convIDParam(c, h.store)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	userID, err := paramUUID(c, "user_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	if err := h.convs.RemoveMember(c.UserContext(), id.UserID, convID, userID); err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, fiber.Map{"removed": true})
}

// ---- helper dung chung cho cac handler ----

// convIDParam doc tham so `:id` cua route hoi thoai.
//
// Nhan CA HAI dang id cua cung mot nhom:
//   - UUID noi bo        (vd 47ea176b-1bd0-4740-a878-deaca8a9731d)
//   - chatId cua CMS     (vd wg_00f2d7a7021bdf59) = conversations.external_id
//
// Nho vay /api/v1, /bot/v1 va /api/client/v1 dung chung MOT gia tri id.
// Dang wg_ ton them 1 query tra external_id; dang UUID khong ton gi.
func convIDParam(c *fiber.Ctx, store repository.Store) (uuid.UUID, error) {
	raw := strings.TrimSpace(c.Params("id"))
	if raw == "" {
		return uuid.Nil, apperr.Invalid("invalid_id", "Thieu id hoi thoai")
	}
	if id, err := uuid.Parse(raw); err == nil {
		return id, nil
	}
	if store == nil {
		return uuid.Nil, apperr.Invalid("invalid_uuid", "id phai la UUID")
	}
	conv, err := store.Conversations().GetByExternalID(c.UserContext(), raw)
	if err != nil {
		return uuid.Nil, err
	}
	return conv.ID, nil
}

func paramUUID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid_uuid", name+" phai la UUID")
	}
	return id, nil
}

func parseUUIDList(in []string, field string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(in))
	for _, s := range in {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, apperr.Invalid("invalid_uuid", field+" chua UUID khong hop le: "+s)
		}
		out = append(out, id)
	}
	return out, nil
}

func pageFromQuery(c *fiber.Ctx) paging.Page {
	return paging.Normalize(c.Query("cursor"), int32(c.QueryInt("limit", 0)))
}

// conversationFilterFromQuery doc ?type=&source=&q= . Rong = khong loc.
// Gia tri la bi tu choi ngay thay vi tra danh sach rong mot cach kho hieu.
func conversationFilterFromQuery(c *fiber.Ctx) (repository.ConversationFilter, error) {
	f := repository.ConversationFilter{Query: c.Query("q")}

	if v := c.Query("type"); v != "" {
		t := domain.ConversationType(v)
		if !t.Valid() {
			return f, apperr.Invalid("invalid_type", "type chi nhan: direct | group")
		}
		f.Type = t
	}
	if v := c.Query("source"); v != "" {
		if v != domain.SourceCMS && v != domain.SourceLocal {
			return f, apperr.Invalid("invalid_source", "source chi nhan: cms | local")
		}
		f.Source = v
	}
	return f, nil
}
