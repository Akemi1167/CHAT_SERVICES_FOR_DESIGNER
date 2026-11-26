package message

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// SidebarItem: 1 dong tren thanh ben kieu Lark — nhom hoac thread, tron chung.
type SidebarItem struct {
	Kind           repository.SidebarKind
	Conversation   *domain.Conversation // group: chinh nhom; thread: nhom chua thread
	LastActivityAt time.Time
	// LastMessage: group = tin GOC moi nhat; thread = tin moi nhat trong thread
	// (chua co tin nao => tin goc). nil = nhom chua co tin nao.
	LastMessage *domain.Message

	// Chi group:
	UnreadCount  int64
	MentionCount int64      // tin chua doc nhac toi minh (dau "@")
	PinnedAt     *time.Time // nguoi xem ghim nhom nay (nil = khong ghim)

	// Chi thread:
	Root           *domain.Message // tin goc (card don / tin co nguoi tra loi), ThreadCount da gan
	ThreadCount    int64           // moi tin trong thread
	UserReplyCount int64           // chi tin nguoi dung; 0 => "Chua co phan hoi"
}

// ListSidebar: nhom + thread tron chung, moi hoat dong nhat truoc (lazy load bang cursor).
// So query co dinh cho ca trang (danh sach, tin goc, tin cuoi thread, tin cuoi nhom,
// unread) + 1 query moi NHOM khac nhau — khong N+1 theo so dong.
//
// Nhom nguoi xem da ghim KHONG nam o day — lay rieng bang ListPinnedSidebar.
func (s *Service) ListSidebar(ctx context.Context, actorID uuid.UUID, p paging.Page) ([]SidebarItem, paging.PageInfo, error) {
	rows, err := s.store.Messages().ListSidebar(ctx,
		repository.SidebarFilter{UserID: actorID, ExcludePinnedGroups: true}, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	rows, info := paging.Build(rows, p, func(r repository.SidebarRow) (time.Time, string) {
		return r.LastActivityAt, r.ID.String()
	})
	items, err := s.buildSidebar(ctx, actorID, rows)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	return items, info, nil
}

// ListPinnedSidebar: cac nhom nguoi xem da ghim, ghim gan nhat truoc (khong phan trang).
func (s *Service) ListPinnedSidebar(ctx context.Context, actorID uuid.UUID) ([]SidebarItem, error) {
	rows, err := s.store.Messages().ListPinnedSidebar(ctx, actorID)
	if err != nil {
		return nil, err
	}
	return s.buildSidebar(ctx, actorID, rows)
}

// buildSidebar gan tin goc / tin cuoi / unread cho 1 trang dong thanh ben.
func (s *Service) buildSidebar(ctx context.Context, actorID uuid.UUID, rows []repository.SidebarRow) ([]SidebarItem, error) {
	if len(rows) == 0 {
		return []SidebarItem{}, nil
	}

	var rootIDs, groupIDs, convIDs []uuid.UUID
	seenConv := map[uuid.UUID]bool{}
	for _, r := range rows {
		if r.Kind == repository.SidebarThread {
			rootIDs = append(rootIDs, r.ID)
		} else {
			groupIDs = append(groupIDs, r.ID)
		}
		if !seenConv[r.ConversationID] {
			seenConv[r.ConversationID] = true
			convIDs = append(convIDs, r.ConversationID)
		}
	}

	roots, err := s.store.Messages().ListByIDs(ctx, rootIDs)
	if err != nil {
		return nil, err
	}
	rootByID := make(map[uuid.UUID]*domain.Message, len(roots))
	for i := range roots {
		rootByID[roots[i].ID] = &roots[i]
	}
	lastInThread, err := s.store.Messages().LastInThreads(ctx, rootIDs)
	if err != nil {
		return nil, err
	}
	lastInGroup, err := s.store.Messages().LastRootMessages(ctx, groupIDs)
	if err != nil {
		return nil, err
	}
	unread, err := s.store.Conversations().UnreadCounts(ctx, actorID, groupIDs)
	if err != nil {
		return nil, err
	}
	mentions, err := s.convs.MentionCounts(ctx, actorID, groupIDs)
	if err != nil {
		return nil, err
	}
	convByID := make(map[uuid.UUID]*domain.Conversation, len(convIDs))
	for _, id := range convIDs {
		c, err := s.store.Conversations().GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		convByID[id] = c
	}

	out := make([]SidebarItem, 0, len(rows))
	for _, r := range rows {
		item := SidebarItem{
			Kind: r.Kind, Conversation: convByID[r.ConversationID], LastActivityAt: r.LastActivityAt,
		}
		switch r.Kind {
		case repository.SidebarThread:
			root, ok := rootByID[r.ID]
			if !ok {
				continue // tin goc vua bi xoa giua 2 query
			}
			root.ThreadCount = r.ThreadCount
			item.Root, item.ThreadCount, item.UserReplyCount = root, r.ThreadCount, r.UserReplyCount
			item.LastMessage = root
			if last, ok := lastInThread[r.ID]; ok {
				item.LastMessage = &last
			}
		default:
			item.UnreadCount = unread[r.ID]
			item.MentionCount = mentions[r.ID]
			item.PinnedAt = r.PinnedAt
			if last, ok := lastInGroup[r.ID]; ok {
				item.LastMessage = &last
			}
		}
		out = append(out, item)
	}
	return out, nil
}
