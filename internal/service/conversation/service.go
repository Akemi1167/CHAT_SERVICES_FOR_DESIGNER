// Package conversation xu ly nghiep vu chat rieng / chat nhom.
package conversation

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type Service struct {
	store repository.Store
	hub   *realtime.Hub
}

func New(store repository.Store, hub *realtime.Hub) *Service {
	return &Service{store: store, hub: hub}
}

type CreateInput struct {
	ActorID       uuid.UUID
	Type          domain.ConversationType
	Name          string
	AvatarURL     string
	MemberUserIDs []uuid.UUID
	Metadata      domain.JSONMap
}

// Create tao chat rieng (idempotent theo cap user) hoac chat nhom.
func (s *Service) Create(ctx context.Context, in CreateInput) (*domain.Conversation, error) {
	if !in.Type.Valid() {
		return nil, apperr.Invalid("invalid_conversation_type", "type phai la direct hoac group")
	}

	memberIDs := uniqueIDs(append(in.MemberUserIDs, in.ActorID))

	if in.Type == domain.ConversationDirect {
		if len(memberIDs) != 2 {
			return nil, apperr.Invalid("invalid_direct_members", "Chat rieng can dung 2 nguoi")
		}
		key := domain.BuildDirectKey(memberIDs[0], memberIDs[1])
		// Da ton tai -> tra lai luon (tranh tao trung).
		if existing, err := s.store.Conversations().GetByDirectKey(ctx, key); err == nil {
			return s.decorate(ctx, in.ActorID, existing)
		} else if !isNotFound(err) {
			return nil, err
		}

		convID := uuid.New()
		conv := &domain.Conversation{
			ID:        convID,
			Type:      domain.ConversationDirect,
			DirectKey: &key,
			CreatedBy: in.ActorID,
			Metadata:  in.Metadata,
			// Chat rieng: 2 ben ngang quyen.
			Members: buildMembers(convID, memberIDs, uuid.Nil, domain.MemberNormal),
		}
		if err := s.store.Conversations().Create(ctx, conv); err != nil {
			// Race: 2 request tao cung luc -> doc lai ban ghi da co.
			if apperr.From(err).Kind == apperr.KindConflict {
				if existing, e := s.store.Conversations().GetByDirectKey(ctx, key); e == nil {
					return s.decorate(ctx, in.ActorID, existing)
				}
			}
			return nil, err
		}
		return s.afterCreate(ctx, in.ActorID, conv.ID, memberIDs)
	}

	// Group
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, apperr.Invalid("group_name_required", "Chat nhom phai co ten")
	}
	convID := uuid.New()
	conv := &domain.Conversation{
		ID:        convID,
		Type:      domain.ConversationGroup,
		Name:      name,
		AvatarURL: in.AvatarURL,
		CreatedBy: in.ActorID,
		Metadata:  in.Metadata,
		// Nguoi tao nhom la owner, nhung nguoi con lai la member.
		Members: buildMembers(convID, memberIDs, in.ActorID, domain.MemberNormal),
	}
	if err := s.store.Conversations().Create(ctx, conv); err != nil {
		return nil, err
	}
	return s.afterCreate(ctx, in.ActorID, conv.ID, memberIDs)
}

func (s *Service) afterCreate(ctx context.Context, actorID, convID uuid.UUID, memberIDs []uuid.UUID) (*domain.Conversation, error) {
	conv, err := s.store.Conversations().GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	s.hub.Publish(ctx, realtime.NewEvent(
		realtime.EventConversationCreated, convID.String(), memberIDs, conv,
	))
	return s.decorate(ctx, actorID, conv)
}

// Get kiem tra quyen truy cap roi tra conversation + so tin chua doc.
func (s *Service) Get(ctx context.Context, actorID, convID uuid.UUID) (*domain.Conversation, error) {
	if _, err := s.EnsureMember(ctx, convID, actorID); err != nil {
		return nil, err
	}
	conv, err := s.store.Conversations().GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	return s.decorate(ctx, actorID, conv)
}

// List tra hoi thoai cua actorID. ConversationFilter rong = khong loc gi.
func (s *Service) List(ctx context.Context, actorID uuid.UUID, f repository.ConversationFilter, p paging.Page) ([]domain.Conversation, paging.PageInfo, error) {
	rows, err := s.store.Conversations().ListByUser(ctx, actorID, f, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, func(c domain.Conversation) (time.Time, string) {
		at := c.CreatedAt
		if c.LastMessageAt != nil {
			at = *c.LastMessageAt
		}
		return at, c.ID.String()
	})

	ids := make([]uuid.UUID, 0, len(items))
	for _, c := range items {
		ids = append(ids, c.ID)
	}
	unread, err := s.store.Conversations().UnreadCounts(ctx, actorID, ids)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	for i := range items {
		items[i].UnreadCount = unread[items[i].ID]
	}
	return items, info, nil
}

// EnsureMember la "cong kiem soat quyen" dung chung cho message/bot service.
func (s *Service) EnsureMember(ctx context.Context, convID, userID uuid.UUID) (*domain.ConversationMember, error) {
	m, err := s.store.Conversations().GetMember(ctx, convID, userID)
	if err != nil {
		if isNotFound(err) {
			return nil, apperr.ErrNotMember
		}
		return nil, err
	}
	return m, nil
}

type AddMembersInput struct {
	ActorID uuid.UUID
	ConvID  uuid.UUID
	UserIDs []uuid.UUID
	Role    domain.MemberRole
}

func (s *Service) AddMembers(ctx context.Context, in AddMembersInput) (*domain.Conversation, error) {
	actor, err := s.EnsureMember(ctx, in.ConvID, in.ActorID)
	if err != nil {
		return nil, err
	}
	conv, err := s.store.Conversations().GetByID(ctx, in.ConvID)
	if err != nil {
		return nil, err
	}
	if conv.Type == domain.ConversationDirect {
		return nil, apperr.Invalid("direct_cannot_add_member", "Khong them nguoi vao chat rieng, hay tao nhom")
	}
	if actor.Role != domain.MemberOwner && actor.Role != domain.MemberAdmin {
		return nil, apperr.Forbidden("require_admin", "Chi owner/admin duoc them thanh vien")
	}
	role := in.Role
	if !role.Valid() {
		role = domain.MemberNormal
	}
	members := buildMembers(in.ConvID, uniqueIDs(in.UserIDs), uuid.Nil, role)
	if len(members) == 0 {
		return nil, apperr.Invalid("members_required", "Phai co it nhat 1 user_id")
	}
	if err := s.store.Conversations().AddMembers(ctx, members); err != nil {
		return nil, err
	}

	updated, err := s.store.Conversations().GetByID(ctx, in.ConvID)
	if err != nil {
		return nil, err
	}
	s.hub.Publish(ctx, realtime.NewEvent(
		realtime.EventConversationUpdated, in.ConvID.String(), memberIDsOf(updated), updated,
	))
	return s.decorate(ctx, in.ActorID, updated)
}

func (s *Service) RemoveMember(ctx context.Context, actorID, convID, userID uuid.UUID) error {
	actor, err := s.EnsureMember(ctx, convID, actorID)
	if err != nil {
		return err
	}
	if actorID != userID && actor.Role != domain.MemberOwner && actor.Role != domain.MemberAdmin {
		return apperr.Forbidden("require_admin", "Chi owner/admin duoc xoa thanh vien")
	}
	conv, err := s.store.Conversations().GetByID(ctx, convID)
	if err != nil {
		return err
	}
	recipients := memberIDsOf(conv)
	if err := s.store.Conversations().RemoveMember(ctx, convID, userID); err != nil {
		return err
	}
	s.hub.Publish(ctx, realtime.NewEvent(
		realtime.EventConversationUpdated, convID.String(), recipients,
		map[string]any{"conversation_id": convID, "removed_user_id": userID},
	))
	return nil
}

// MemberUserIDs dung cho message service de tinh recipients cua event.
func (s *Service) MemberUserIDs(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error) {
	return s.store.Conversations().ListMemberUserIDs(ctx, convID)
}

func (s *Service) decorate(ctx context.Context, actorID uuid.UUID, conv *domain.Conversation) (*domain.Conversation, error) {
	counts, err := s.store.Conversations().UnreadCounts(ctx, actorID, []uuid.UUID{conv.ID})
	if err != nil {
		return nil, err
	}
	conv.UnreadCount = counts[conv.ID]
	return conv, nil
}

// buildMembers luon gan ConversationID: thieu no thi INSERT se vi pham khoa ngoai.
func buildMembers(convID uuid.UUID, ids []uuid.UUID, ownerID uuid.UUID, defRole domain.MemberRole) []domain.ConversationMember {
	out := make([]domain.ConversationMember, 0, len(ids))
	for _, id := range ids {
		role := defRole
		if ownerID != uuid.Nil && id == ownerID {
			role = domain.MemberOwner
		}
		out = append(out, domain.ConversationMember{
			ID:             uuid.New(),
			ConversationID: convID,
			UserID:         id,
			Role:           role,
		})
	}
	return out
}

func memberIDsOf(c *domain.Conversation) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(c.Members))
	for _, m := range c.Members {
		out = append(out, m.UserID)
	}
	return out
}

func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func isNotFound(err error) bool {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Kind == apperr.KindNotFound
	}
	return false
}
