package conversation

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/realtime"
)

// SetPinned ghim / bo ghim nhom len dau thanh ben cho RIENG actorID (nguoi khac
// khong bi anh huong). Ghim lai nhom da ghim = dua no len tren cung (pinnedAt moi).
// Tra thoi diem ghim, nil khi bo ghim.
func (s *Service) SetPinned(ctx context.Context, actorID, convID uuid.UUID, pinned bool) (*time.Time, error) {
	if _, err := s.EnsureMember(ctx, convID, actorID); err != nil {
		return nil, err
	}
	var at *time.Time
	if pinned {
		now := time.Now().UTC()
		at = &now
	}
	if err := s.store.Conversations().SetPinned(ctx, convID, actorID, at); err != nil {
		return nil, err
	}

	// Chi bao cho chinh nguoi do: cac tab / thiet bi khac cua ho sap xep lai thanh ben.
	chatID := ""
	if conv, err := s.store.Conversations().GetMeta(ctx, convID); err == nil && conv.ExternalID != nil {
		chatID = *conv.ExternalID
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventReload, convID.String(), []uuid.UUID{actorID}, map[string]any{
		"reason":          "group.pinned",
		"conversation_id": convID,
		"chat_id":         chatID,
		"pinned":          pinned,
		"pinned_at":       at,
	}))
	return at, nil
}
