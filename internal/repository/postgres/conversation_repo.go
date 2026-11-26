package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type ConversationRepo struct{ base }

var _ repository.ConversationRepository = (*ConversationRepo)(nil)

func (r *ConversationRepo) Create(ctx context.Context, c *domain.Conversation) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	for i := range c.Members {
		if c.Members[i].ID == uuid.Nil {
			c.Members[i].ID = uuid.New()
		}
		c.Members[i].ConversationID = c.ID
		if c.Members[i].JoinedAt.IsZero() {
			c.Members[i].JoinedAt = time.Now().UTC()
		}
	}
	return wrap(r.db(ctx).Create(c).Error, "conversation.create")
}

func (r *ConversationRepo) GetMeta(ctx context.Context, id uuid.UUID) (*domain.Conversation, error) {
	var c domain.Conversation
	if err := r.db(ctx).First(&c, "id = ?", id).Error; err != nil {
		return nil, mapErr(err, apperr.ErrConvNotFound)
	}
	return &c, nil
}

func (r *ConversationRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.db(ctx).
		Preload("Members", func(db *gorm.DB) *gorm.DB {
			return db.Where("left_at IS NULL")
		}).
		Preload("Members.User").
		First(&c, "id = ?", id).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrConvNotFound)
	}
	return &c, nil
}

func (r *ConversationRepo) GetByDirectKey(ctx context.Context, key string) (*domain.Conversation, error) {
	var c domain.Conversation
	err := r.db(ctx).
		Preload("Members").Preload("Members.User").
		First(&c, "direct_key = ?", key).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrConvNotFound)
	}
	return &c, nil
}

// ListByUser tra cac hoi thoai user dang tham gia, moi nhat truoc.
// Sort key = COALESCE(last_message_at, created_at) de hoi thoai moi tao van len dau.
func (r *ConversationRepo) ListByUser(ctx context.Context, userID uuid.UUID, f repository.ConversationFilter, p paging.Page) ([]domain.Conversation, error) {
	q := r.db(ctx).Model(&domain.Conversation{}).
		Joins("JOIN conversation_members cm ON cm.conversation_id = conversations.id AND cm.left_at IS NULL").
		Where("cm.user_id = ?", userID)
	q = applyConversationFilter(q, f)

	if p.Cursor.Valid {
		q = q.Where(
			"(COALESCE(conversations.last_message_at, conversations.created_at), conversations.id) < (?, ?)",
			p.Cursor.CreatedAt, p.Cursor.ID,
		)
	}

	var out []domain.Conversation
	err := q.Preload("Members", func(db *gorm.DB) *gorm.DB { return db.Where("left_at IS NULL") }).
		Preload("Members.User").
		Order("COALESCE(conversations.last_message_at, conversations.created_at) DESC, conversations.id DESC").
		Limit(p.FetchLimit()).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "conversation.listByUser")
	}
	return out, nil
}

func (r *ConversationRepo) UpdateLastMessage(ctx context.Context, convID, msgID uuid.UUID, at time.Time) error {
	err := r.db(ctx).Model(&domain.Conversation{}).
		Where("id = ?", convID).
		Updates(map[string]any{
			"last_message_id": msgID,
			"last_message_at": at,
			"updated_at":      time.Now().UTC(),
		}).Error
	return wrap(err, "conversation.updateLastMessage")
}

func (r *ConversationRepo) AddMembers(ctx context.Context, members []domain.ConversationMember) error {
	if len(members) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for i := range members {
		if members[i].ConversationID == uuid.Nil || members[i].UserID == uuid.Nil {
			return apperr.Invalid("invalid_member",
				"Thieu conversation_id hoac user_id khi them thanh vien")
		}
		if members[i].ID == uuid.Nil {
			members[i].ID = uuid.New()
		}
		if members[i].JoinedAt.IsZero() {
			members[i].JoinedAt = now
		}
		if members[i].Role == "" {
			members[i].Role = domain.MemberNormal
		}
	}
	// Re-join: cap nhat left_at = NULL thay vi tao ban ghi moi.
	err := r.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{"left_at": nil, "joined_at": now}),
	}).Create(&members).Error
	return wrap(err, "conversation.addMembers")
}

func (r *ConversationRepo) RemoveMember(ctx context.Context, convID, userID uuid.UUID) error {
	err := r.db(ctx).Model(&domain.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", convID, userID).
		Update("left_at", time.Now().UTC()).Error
	return wrap(err, "conversation.removeMember")
}

func (r *ConversationRepo) GetMember(ctx context.Context, convID, userID uuid.UUID) (*domain.ConversationMember, error) {
	var m domain.ConversationMember
	err := r.db(ctx).Debug().
		First(&m, "conversation_id = ? AND user_id = ? AND left_at IS NULL", convID, userID).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrNotMember)
	}
	return &m, nil
}

func (r *ConversationRepo) ListMembers(ctx context.Context, convID uuid.UUID) ([]domain.ConversationMember, error) {
	var out []domain.ConversationMember
	err := r.db(ctx).Preload("User").
		Where("conversation_id = ? AND left_at IS NULL", convID).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "conversation.listMembers")
	}
	return out, nil
}

func (r *ConversationRepo) ListMemberUserIDs(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db(ctx).Model(&domain.ConversationMember{}).
		Where("conversation_id = ? AND left_at IS NULL", convID).
		Pluck("user_id", &ids).Error
	if err != nil {
		return nil, wrap(err, "conversation.listMemberUserIDs")
	}
	return ids, nil
}

// MarkRead chi tien len (khong bao gio lui lai moc da doc).
func (r *ConversationRepo) MarkRead(ctx context.Context, convID, userID, msgID uuid.UUID, at time.Time) error {
	err := r.db(ctx).Model(&domain.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", convID, userID).
		Where("last_read_at IS NULL OR last_read_at < ?", at).
		Updates(map[string]any{"last_read_message_id": msgID, "last_read_at": at}).Error
	return wrap(err, "conversation.markRead")
}

type unreadRow struct {
	ConversationID uuid.UUID
	Cnt            int64
}

// UnreadCounts dem tin nhan chua doc cho tung conversation (bo qua tin cua chinh minh).
func (r *ConversationRepo) UnreadCounts(ctx context.Context, userID uuid.UUID, convIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	result := make(map[uuid.UUID]int64, len(convIDs))
	if len(convIDs) == 0 {
		return result, nil
	}
	var rows []unreadRow
	err := r.db(ctx).Raw(`
		SELECT m.conversation_id AS conversation_id, COUNT(*) AS cnt
		FROM messages m
		JOIN conversation_members cm
		  ON cm.conversation_id = m.conversation_id AND cm.user_id = ? AND cm.left_at IS NULL
		WHERE m.conversation_id IN ?
		  AND m.deleted_at IS NULL
		  AND (cm.last_read_at IS NULL OR m.created_at > cm.last_read_at)
		  AND (m.sender_id IS NULL OR m.sender_id <> ?)
		GROUP BY m.conversation_id`, userID, convIDs, userID).Scan(&rows).Error
	if err != nil {
		return nil, wrap(err, "conversation.unreadCounts")
	}
	for _, row := range rows {
		result[row.ConversationID] = row.Cnt
	}
	return result, nil
}

func (r *ConversationRepo) SetPinned(ctx context.Context, convID, userID uuid.UUID, at *time.Time) error {
	res := r.db(ctx).Model(&domain.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ? AND left_at IS NULL", convID, userID).
		Update("pinned_at", at)
	if res.Error != nil {
		return wrap(res.Error, "conversation.setPinned")
	}
	if res.RowsAffected == 0 {
		return apperr.ErrNotMember
	}
	return nil
}

// MentionCounts dem tin chua doc nhac toi user (dau "@" tren danh sach nhom).
// Dieu kien "chua doc" giong het UnreadCounts; dung index GIN ix_messages_mentions.
func (r *ConversationRepo) MentionCounts(ctx context.Context, userID uuid.UUID, mentionKey string, convIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	result := make(map[uuid.UUID]int64, len(convIDs))
	if len(convIDs) == 0 || mentionKey == "" {
		return result, nil
	}
	needle, err := json.Marshal([]string{mentionKey})
	if err != nil {
		return nil, wrap(err, "conversation.mentionCounts")
	}
	var rows []unreadRow
	err = r.db(ctx).Raw(`
		SELECT m.conversation_id AS conversation_id, COUNT(*) AS cnt
		FROM messages m
		JOIN conversation_members cm
		  ON cm.conversation_id = m.conversation_id AND cm.user_id = ? AND cm.left_at IS NULL
		WHERE m.conversation_id IN ?
		  AND m.deleted_at IS NULL
		  AND m.mentions IS NOT NULL
		  AND m.mentions @> ?::jsonb
		  AND (cm.last_read_at IS NULL OR m.created_at > cm.last_read_at)
		  AND (m.sender_id IS NULL OR m.sender_id <> ?)
		GROUP BY m.conversation_id`, userID, convIDs, string(needle), userID).Scan(&rows).Error
	if err != nil {
		return nil, wrap(err, "conversation.mentionCounts")
	}
	for _, row := range rows {
		result[row.ConversationID] = row.Cnt
	}
	return result, nil
}

// ListPeerUserIDs: nhung nguoi chung it nhat 1 hoi thoai dang hieu luc voi user nay
// (ke ca chinh user). Dung de phat presence dung pham vi, khong broadcast toan he thong.
func (r *ConversationRepo) ListPeerUserIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.db(ctx).Raw(`
		SELECT DISTINCT peer.user_id
		FROM conversation_members me
		JOIN conversation_members peer
		  ON peer.conversation_id = me.conversation_id AND peer.left_at IS NULL
		WHERE me.user_id = ? AND me.left_at IS NULL`, userID).Scan(&ids).Error
	if err != nil {
		return nil, wrap(err, "conversation.listPeerUserIDs")
	}
	return ids, nil
}

// applyConversationFilter gan dieu kien loc; field rong thi bo qua.
// Phai goi truoc khi them dieu kien cursor de thu tu WHERE khong doi y nghia.
func applyConversationFilter(q *gorm.DB, f repository.ConversationFilter) *gorm.DB {
	if f.Type != "" {
		q = q.Where("conversations.type = ?", string(f.Type))
	}
	if f.Source != "" {
		q = q.Where("conversations.source = ?", f.Source)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		// ILIKE: Postgres, khong phan biet hoa thuong. Escape ky tu dai dien
		// de nguoi dung go "%" khong quet ca bang.
		q = q.Where("conversations.name ILIKE ?", "%"+escapeLike(s)+"%")
	}
	return q
}

// escapeLike vo hieu hoa %, _ va \\ trong chuoi nguoi dung nhap.
func escapeLike(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_")
	return r.Replace(s)
}
