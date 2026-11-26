package postgres

import (
	"context"
	"errors"
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

type MessageRepo struct{ base }

var _ repository.MessageRepository = (*MessageRepo)(nil)

func (r *MessageRepo) Create(ctx context.Context, m *domain.Message) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	for i := range m.Attachments {
		if m.Attachments[i].ID == uuid.Nil {
			m.Attachments[i].ID = uuid.New()
		}
		m.Attachments[i].MessageID = m.ID
	}
	return wrap(r.db(ctx).Create(m).Error, "message.create")
}

func (r *MessageRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Message, error) {
	var m domain.Message
	err := r.db(ctx).Preload("Attachments").Preload("Sender").
		First(&m, "id = ?", id).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrMessageNotFound)
	}
	return &m, nil
}

func (r *MessageRepo) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Message, error) {
	var out []domain.Message
	if len(ids) == 0 {
		return out, nil
	}
	if err := r.db(ctx).Preload("Attachments").Preload("Sender").
		Where("id IN ?", ids).Find(&out).Error; err != nil {
		return nil, wrap(err, "message.listByIDs")
	}
	return out, nil
}

// ListSidebar tron nhom va thread thanh 1 danh sach (1 query, khong N+1):
//
//	agg     = dem / lay luc moi nhat cua tin trong thread, theo thread_id
//	threads = tin goc la card HOAC co it nhat 1 tin trong thread
//	groups  = nhom cua toi; hoat dong = tin GOC moi nhat (chua co tin => luc tao nhom)
func (r *MessageRepo) ListSidebar(ctx context.Context, f repository.SidebarFilter, p paging.Page) ([]repository.SidebarRow, error) {
	args := map[string]any{"uid": f.UserID, "limit": p.FetchLimit()}
	cursor := ""
	if p.Cursor.Valid {
		cursor = `WHERE (f.last_activity_at, f.id) < (@cur_at, @cur_id)`
		args["cur_at"], args["cur_id"] = p.Cursor.CreatedAt, p.Cursor.ID
	}

	pinFilter := ""
	if f.ExcludePinnedGroups {
		pinFilter = " AND my.pinned_at IS NULL"
	}

	q := `
WITH my AS (
	SELECT conversation_id, pinned_at FROM conversation_members
	 WHERE user_id = @uid AND left_at IS NULL
),
agg AS (
	SELECT t.thread_id                                    AS root_id,
	       COUNT(*)                                       AS thread_count,
	       COUNT(*) FILTER (WHERE t.sender_type = 'user') AS user_reply_count,
	       MAX(t.created_at)                              AS last_at
	  FROM messages t
	 WHERE t.thread_id IS NOT NULL AND t.deleted_at IS NULL
	   AND t.conversation_id IN (SELECT conversation_id FROM my)
	 GROUP BY t.thread_id
),
threads AS (
	SELECT 'thread'                          AS kind,
	       r.id                              AS id,
	       r.conversation_id                 AS conversation_id,
	       COALESCE(a.last_at, r.created_at) AS last_activity_at,
	       COALESCE(a.thread_count, 0)       AS thread_count,
	       COALESCE(a.user_reply_count, 0)   AS user_reply_count,
	       NULL::timestamptz                 AS pinned_at
	  FROM messages r
	  LEFT JOIN agg a ON a.root_id = r.id
	 WHERE r.conversation_id IN (SELECT conversation_id FROM my)
	   AND r.thread_id IS NULL AND r.deleted_at IS NULL
	   AND (r.card IS NOT NULL OR a.root_id IS NOT NULL)
),
groups AS (
	SELECT 'group' AS kind,
	       c.id    AS id,
	       c.id    AS conversation_id,
	       COALESCE((SELECT MAX(m.created_at) FROM messages m
	                  WHERE m.conversation_id = c.id
	                    AND m.thread_id IS NULL AND m.deleted_at IS NULL),
	                c.created_at) AS last_activity_at,
	       0 AS thread_count,
	       0 AS user_reply_count,
	       my.pinned_at AS pinned_at
	  FROM conversations c
	  JOIN my ON my.conversation_id = c.id` + pinFilter + `
)
SELECT * FROM (SELECT * FROM threads UNION ALL SELECT * FROM groups) f
` + cursor + `
 ORDER BY f.last_activity_at DESC, f.id DESC
 LIMIT @limit`

	var out []repository.SidebarRow
	if err := r.db(ctx).Raw(q, args).Scan(&out).Error; err != nil {
		return nil, wrap(err, "message.listSidebar")
	}
	return out, nil
}

func (r *MessageRepo) ListPinnedSidebar(ctx context.Context, userID uuid.UUID) ([]repository.SidebarRow, error) {
	var out []repository.SidebarRow
	err := r.db(ctx).Raw(`
SELECT 'group' AS kind,
       c.id    AS id,
       c.id    AS conversation_id,
       COALESCE((SELECT MAX(m.created_at) FROM messages m
                  WHERE m.conversation_id = c.id
                    AND m.thread_id IS NULL AND m.deleted_at IS NULL),
                c.created_at) AS last_activity_at,
       0 AS thread_count,
       0 AS user_reply_count,
       cm.pinned_at AS pinned_at
  FROM conversation_members cm
  JOIN conversations c ON c.id = cm.conversation_id
 WHERE cm.user_id = ? AND cm.left_at IS NULL AND cm.pinned_at IS NOT NULL
 ORDER BY cm.pinned_at DESC, c.id DESC`, userID).Scan(&out).Error
	if err != nil {
		return nil, wrap(err, "message.listPinnedSidebar")
	}
	return out, nil
}

func (r *MessageRepo) LastRootMessages(ctx context.Context, convIDs []uuid.UUID) (map[uuid.UUID]domain.Message, error) {
	out := make(map[uuid.UUID]domain.Message, len(convIDs))
	if len(convIDs) == 0 {
		return out, nil
	}
	var rows []domain.Message
	err := r.db(ctx).Preload("Sender").
		Select("DISTINCT ON (conversation_id) *").
		Where("conversation_id IN ? AND thread_id IS NULL", convIDs).
		Order("conversation_id, created_at DESC, id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, wrap(err, "message.lastRootMessages")
	}
	for _, m := range rows {
		out[m.ConversationID] = m
	}
	return out, nil
}

func (r *MessageRepo) LastInThreads(ctx context.Context, rootIDs []uuid.UUID) (map[uuid.UUID]domain.Message, error) {
	out := make(map[uuid.UUID]domain.Message, len(rootIDs))
	if len(rootIDs) == 0 {
		return out, nil
	}
	var rows []domain.Message
	err := r.db(ctx).Preload("Sender").
		Select("DISTINCT ON (thread_id) *").
		Where("thread_id IN ?", rootIDs).
		Order("thread_id, created_at DESC, id DESC").
		Find(&rows).Error
	if err != nil {
		return nil, wrap(err, "message.lastInThreads")
	}
	for _, m := range rows {
		if m.ThreadID != nil {
			out[*m.ThreadID] = m
		}
	}
	return out, nil
}

func (r *MessageRepo) GetCardByCode(ctx context.Context, code string) (*domain.Message, error) {
	var m domain.Message
	// Bieu thuc phai trung voi index ix_messages_card_code (migration 000007).
	err := r.db(ctx).Preload("Attachments").Preload("Sender").
		Where("card IS NOT NULL AND card -> 'header' ->> 'code' = ?", code).
		Order("created_at DESC").
		First(&m).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrCardNotFound)
	}
	return &m, nil
}

func (r *MessageRepo) Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]repository.MessageLocation, error) {
	out := make(map[uuid.UUID]repository.MessageLocation, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID             uuid.UUID
		ConversationID uuid.UUID
		ThreadID       *uuid.UUID
		ExternalID     *string
	}
	err := r.db(ctx).Table("messages AS m").
		Select("m.id, m.conversation_id, m.thread_id, c.external_id").
		Joins("JOIN conversations c ON c.id = m.conversation_id").
		Where("m.id IN ?", ids).
		Scan(&rows).Error
	if err != nil {
		return nil, wrap(err, "message.locations")
	}
	for _, row := range rows {
		loc := repository.MessageLocation{ConversationID: row.ConversationID, ThreadID: row.ThreadID}
		if row.ExternalID != nil {
			loc.GroupID = *row.ExternalID
		}
		out[row.ID] = loc
	}
	return out, nil
}

// GetByClientMsgID dung cho idempotency: FE retry gui lai cung client_msg_id.
func (r *MessageRepo) GetByClientMsgID(ctx context.Context, convID uuid.UUID, clientMsgID string) (*domain.Message, error) {
	if clientMsgID == "" {
		return nil, apperr.ErrMessageNotFound
	}
	var m domain.Message
	err := r.db(ctx).Preload("Attachments").Preload("Sender").
		First(&m, "conversation_id = ? AND client_msg_id = ?", convID, clientMsgID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperr.ErrMessageNotFound
		}
		return nil, wrap(err, "message.getByClientMsgID")
	}
	return &m, nil
}

// List tra timeline moi-nhat-truoc theo cursor (created_at, id).
func (r *MessageRepo) List(ctx context.Context, f repository.MessageFilter, p paging.Page) ([]domain.Message, error) {
	q := r.db(ctx).Model(&domain.Message{}).
		Where("conversation_id = ?", f.ConversationID)

	if f.ReplyToID != nil {
		q = q.Where("reply_to_id = ?", *f.ReplyToID)
	}

	switch {
	case f.ThreadID != nil:
		// Tin nhan trong thread + chinh message goc.
		q = q.Where("thread_id = ? OR id = ?", *f.ThreadID, *f.ThreadID)
	case f.OnlyRoot:
		q = q.Where("thread_id IS NULL")
	}

	if p.Cursor.Valid {
		q = q.Where("(created_at, id) < (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}

	var out []domain.Message
	err := q.Preload("Attachments").Preload("Sender").
		Order("created_at DESC, id DESC").
		Limit(p.FetchLimit()).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "message.list")
	}
	return out, nil
}

// Search: theo noi dung, ma task (metadata->>'task_code'), nguoi gui, khoang ngay.
func (r *MessageRepo) Search(ctx context.Context, f repository.SearchFilter, p paging.Page) ([]domain.Message, error) {
	q := r.db(ctx).Model(&domain.Message{}).
		Joins(`JOIN conversation_members cm
		       ON cm.conversation_id = messages.conversation_id
		      AND cm.user_id = ? AND cm.left_at IS NULL`, f.UserID)

	if f.ConversationID != uuid.Nil {
		q = q.Where("messages.conversation_id = ?", f.ConversationID)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		like := "%" + s + "%"
		q = q.Where(`(messages.content ILIKE ? OR messages.metadata->>'task_code' ILIKE ?)`, like, like)
	}
	if f.SenderID != nil {
		q = q.Where("messages.sender_id = ?", *f.SenderID)
	}
	if f.FromDate != nil {
		q = q.Where("messages.created_at >= ?", *f.FromDate)
	}
	if f.ToDate != nil {
		q = q.Where("messages.created_at <= ?", *f.ToDate)
	}
	if p.Cursor.Valid {
		q = q.Where("(messages.created_at, messages.id) < (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}

	var out []domain.Message
	err := q.Preload("Attachments").Preload("Sender").
		Order("messages.created_at DESC, messages.id DESC").
		Limit(p.FetchLimit()).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "message.search")
	}
	return out, nil
}

func (r *MessageRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return wrap(r.db(ctx).Delete(&domain.Message{}, "id = ?", id).Error, "message.softDelete")
}

// UpsertReceipts luu "ai da xem tin nhan nao" (hien danh sach nguoi da doc).
func (r *MessageRepo) UpsertReceipts(ctx context.Context, receipts []domain.MessageReceipt) error {
	if len(receipts) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for i := range receipts {
		if receipts[i].SeenAt.IsZero() {
			receipts[i].SeenAt = now
		}
	}
	err := r.db(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "message_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"seen_at"}),
	}).Create(&receipts).Error
	return wrap(err, "message.upsertReceipts")
}

func (r *MessageRepo) ListReceipts(ctx context.Context, messageID uuid.UUID) ([]domain.MessageReceipt, error) {
	var out []domain.MessageReceipt
	err := r.db(ctx).Where("message_id = ?", messageID).Find(&out).Error
	if err != nil {
		return nil, wrap(err, "message.listReceipts")
	}
	return out, nil
}

// ListReplies: thu tu CU -> MOI de doc thread tu tren xuong.
func (r *MessageRepo) ListReplies(ctx context.Context, messageID uuid.UUID, mode repository.ReplyMode, p paging.Page) ([]domain.Message, error) {
	q := r.db(ctx).Model(&domain.Message{})
	switch mode {
	case repository.ReplyThread:
		// Ca thread do tin nay mo (khong gom chinh tin goc).
		q = q.Where("thread_id = ?", messageID)
	default:
		q = q.Where("reply_to_id = ?", messageID)
	}
	if p.Cursor.Valid {
		q = q.Where("(created_at, id) > (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}

	var out []domain.Message
	err := q.Preload("Attachments").Preload("Sender").
		Order("created_at ASC, id ASC").
		Limit(p.FetchLimit()).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "message.listReplies")
	}
	return out, nil
}

type replyCountRow struct {
	ParentID uuid.UUID
	Cnt      int64
}

func (r *MessageRepo) CountReplies(ctx context.Context, messageIDs []uuid.UUID, mode repository.ReplyMode) (map[uuid.UUID]int64, error) {
	out := make(map[uuid.UUID]int64, len(messageIDs))
	if len(messageIDs) == 0 {
		return out, nil
	}
	column := "reply_to_id"
	if mode == repository.ReplyThread {
		column = "thread_id"
	}

	var rows []replyCountRow
	err := r.db(ctx).Model(&domain.Message{}).
		Select(column+" AS parent_id, COUNT(*) AS cnt").
		Where(column+" IN ?", messageIDs).
		Group(column).Scan(&rows).Error
	if err != nil {
		return nil, wrap(err, "message.countReplies")
	}
	for _, row := range rows {
		out[row.ParentID] = row.Cnt
	}
	return out, nil
}

// ---------------- ghim tin nhan ----------------

func (r *MessageRepo) SetPinned(ctx context.Context, messageID uuid.UUID, pinned bool, by *uuid.UUID) error {
	fields := map[string]any{"pinned": pinned, "updated_at": time.Now().UTC()}
	if pinned {
		fields["pinned_at"] = time.Now().UTC()
		fields["pinned_by"] = by
	} else {
		fields["pinned_at"] = nil
		fields["pinned_by"] = nil
	}
	err := r.db(ctx).Model(&domain.Message{}).Where("id = ?", messageID).Updates(fields).Error
	return wrap(err, "message.setPinned")
}

func (r *MessageRepo) ListPinned(ctx context.Context, conversationID uuid.UUID, limit int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []domain.Message
	err := r.db(ctx).Preload("Attachments").Preload("Sender").
		Where("conversation_id = ? AND pinned", conversationID).
		Order("pinned_at DESC").Limit(limit).Find(&out).Error
	if err != nil {
		return nil, wrap(err, "message.listPinned")
	}
	return out, nil
}

// GetAttachment tra tep + tin nhan chua no (de kiem quyen theo hoi thoai).
func (r *MessageRepo) GetAttachment(ctx context.Context, id uuid.UUID) (*domain.Attachment, *domain.Message, error) {
	var a domain.Attachment
	if err := r.db(ctx).First(&a, "id = ?", id).Error; err != nil {
		return nil, nil, mapErr(err, apperr.NotFound("attachment_not_found", "Khong tim thay tep dinh kem"))
	}
	msg, err := r.GetByID(ctx, a.MessageID)
	if err != nil {
		return nil, nil, err
	}
	return &a, msg, nil
}
