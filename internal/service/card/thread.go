package card

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// ThreadView: mot card don + thread cua no, du de ve panel chi tiet don ma
// khong can biet messageId (FE chi co ma don, vd tu thong bao / link).
type ThreadView struct {
	// Card: tin chua card co ma nay (ban moi nhat). Card con (-P1, -E1...) nam
	// TRONG thread cua card goc => Root la card goc, Card la card con.
	Card *domain.Message
	Root *domain.Message
	// Links: cac khoi {"type":"links"} trong card (vd "Google Drive"), DA loc
	// theo visibleTo cua nguoi xem. Rong = CMS chua gui (chua tao xong folder).
	Links []map[string]any
	// Activity: dong dien bien do bot / he thong ghi vao thread (cu -> moi).
	Activity []domain.Message
	// Discussion: tin nguoi dung trao doi trong thread (cu -> moi).
	Discussion  []domain.Message
	ThreadCount int64
	PageInfo    paging.PageInfo
	// ActorID cua nguoi xem — transport dung de loc card / nut.
	ViewerActorID string
}

// ThreadByCode tra card don theo ma (card.header.code) va thread cua no.
// Nguoi xem phai la thanh vien nhom chua card.
func (s *Service) ThreadByCode(ctx context.Context, actorID uuid.UUID, code string, p paging.Page) (*ThreadView, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return nil, apperr.Invalid("card_code_required", "Thieu ma don")
	}

	msg, err := s.store.Messages().GetCardByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, msg.ConversationID, actorID); err != nil {
		return nil, err
	}
	viewer, err := s.store.Users().GetByID(ctx, actorID)
	if err != nil {
		return nil, err
	}

	root := msg
	if msg.ThreadID != nil {
		if root, err = s.store.Messages().GetByID(ctx, *msg.ThreadID); err != nil {
			return nil, err
		}
	}

	rows, err := s.store.Messages().ListReplies(ctx, root.ID, repository.ReplyThread, p)
	if err != nil {
		return nil, err
	}
	items, info := paging.Build(rows, p, func(m domain.Message) (time.Time, string) {
		return m.CreatedAt, m.ID.String()
	})
	counts, err := s.store.Messages().CountReplies(ctx, []uuid.UUID{root.ID}, repository.ReplyThread)
	if err != nil {
		return nil, err
	}

	activity, discussion := splitThread(items)
	return &ThreadView{
		Card:          msg,
		Root:          root,
		Links:         LinkBlocks(FilterForViewer(msg.Card, viewer.ActorID)),
		Activity:      activity,
		Discussion:    discussion,
		ThreadCount:   counts[root.ID],
		PageInfo:      info,
		ViewerActorID: viewer.ActorID,
	}, nil
}

// splitThread tach dong dien bien (bot/system) khoi tin trao doi cua nguoi dung,
// giu nguyen thu tu cu -> moi trong tung nhom.
func splitThread(items []domain.Message) (activity, discussion []domain.Message) {
	activity = make([]domain.Message, 0, len(items))
	discussion = make([]domain.Message, 0, len(items))
	for _, m := range items {
		if m.SenderType == domain.SenderUser {
			discussion = append(discussion, m)
		} else {
			activity = append(activity, m)
		}
	}
	return activity, discussion
}

// LinkBlocks lay moi phan tu {"type":"links"} o cap elements cua card.
// Chat khong dien giai label/url — tra nguyen de FE ve.
func LinkBlocks(c domain.JSONMap) []map[string]any {
	out := []map[string]any{}
	if c == nil {
		return out
	}
	els, _ := c["elements"].([]any)
	for _, e := range els {
		if el, ok := e.(map[string]any); ok && el["type"] == "links" {
			out = append(out, el)
		}
	}
	return out
}
