// Package repository khai bao hop dong truy cap du lieu (port).
// Service layer chi biet interface o day => de mock khi test, de doi implement.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
)

type MessageFilter struct {
	ConversationID uuid.UUID
	ThreadID       *uuid.UUID
	// ReplyToID: chi lay tin tra loi TRUC TIEP mot tin nhan.
	ReplyToID *uuid.UUID
	// OnlyRoot = true: chi lay tin nhan goc (khong lay reply trong thread).
	OnlyRoot bool
}

// SidebarFilter: danh sach chat tren thanh ben kieu Lark — NHOM va THREAD tron
// chung, moi hoat dong nhat truoc.
type SidebarFilter struct {
	UserID uuid.UUID // chi nhom (va thread trong nhom) nguoi nay con la thanh vien
	// ExcludePinnedGroups: bo nhom nguoi nay da ghim (chung nam o danh sach ghim rieng,
	// xem ListPinnedSidebar) — tranh hien 2 lan.
	ExcludePinnedGroups bool
}

type SidebarKind string

const (
	SidebarGroup  SidebarKind = "group"
	SidebarThread SidebarKind = "thread"
)

// SidebarRow: 1 dong tren thanh ben.
//
//	group : ID = conversation id. LastActivityAt = tin GOC moi nhat (tin trong
//	        thread khong day nhom len — chung hien o dong thread rieng).
//	thread: ID = id tin goc. Thread = tin goc co it nhat 1 tin trong thread HOAC
//	        la card (card don luon mo thread, ke ca khi chua ai tra loi).
type SidebarRow struct {
	Kind           SidebarKind
	ID             uuid.UUID
	ConversationID uuid.UUID
	LastActivityAt time.Time
	ThreadCount    int64      // chi thread: moi tin trong thread
	UserReplyCount int64      // chi thread: tin nguoi dung ("Chua co phan hoi" khi = 0)
	PinnedAt       *time.Time // chi group: nguoi xem ghim nhom nay luc nao (nil = khong ghim)
}

// ReplyMode quyet dinh "reply cua mot tin nhan" nghia la gi:
//
//	direct — tin co reply_to_id = tin do (trich dan truc tiep, giong nut Reply)
//	thread — toan bo tin trong thread do tin do mo (thread_id = tin do)
type ReplyMode string

const (
	ReplyDirect ReplyMode = "direct"
	ReplyThread ReplyMode = "thread"
)

type SearchFilter struct {
	ConversationID uuid.UUID
	// Neu ConversationID = Nil => tim trong moi conversation user la member.
	UserID   uuid.UUID
	Query    string
	SenderID *uuid.UUID
	FromDate *time.Time
	ToDate   *time.Time
}

// ConversationFilter: moi field rong = khong filter.
type ConversationFilter struct {
	Type   domain.ConversationType // group | direct
	Source string                  // cms | local
	Query  string                  // tim trong ten nhom, khong phan biet hoa thuong
}

// NotificationFilter: moi field zero = khong filter.
type NotificationFilter struct {
	UnreadOnly bool
}

type SubmissionFilter struct {
	BotID          *uuid.UUID
	ConversationID *uuid.UUID
	UserID         *uuid.UUID
	Status         *domain.SubmissionStatus
}

type UserRepository interface {
	UpsertByExternalID(ctx context.Context, u *domain.User) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.User, error)
	UpdatePresence(ctx context.Context, id uuid.UUID, p domain.Presence, seenAt time.Time) error
	// UpdatePresenceWithReason dung cho PUT /bot/v1/presence (CMS bao OFF kem ly do).
	UpdatePresenceWithReason(ctx context.Context, id uuid.UUID, p domain.Presence, reason string, seenAt time.Time) error
	// TouchLastSeen lam tuoi last_seen_at cho nhung nguoi dang mo WebSocket.
	// presence.Service goi dinh ky; nho vay khi mot instance chet thi
	// last_seen_at cu dan va User.LivePresence() tu coi la offline.
	TouchLastSeen(ctx context.Context, ids []uuid.UUID, seenAt time.Time) (int64, error)

	// ---- dinh danh phia CMS ----
	// UpsertFromCMS khop theo cms_user_id -> open_id -> external_id, roi cap nhat ho so.
	UpsertFromCMS(ctx context.Context, u *domain.User) (*domain.User, error)
	GetByCMSUserID(ctx context.Context, cmsUserID int64) (*domain.User, error)
	GetByOpenID(ctx context.Context, openID string) (*domain.User, error)
	GetByActorID(ctx context.Context, actorID string) (*domain.User, error)
	ListByOpenIDs(ctx context.Context, openIDs []string) ([]domain.User, error)
	ListByCMSUserIDs(ctx context.Context, ids []int64) ([]domain.User, error)
	ListByActorIDs(ctx context.Context, actorIDs []string) ([]domain.User, error)
	// ListCMSUserIDs tra cms_user_id cua moi user da dong bo tu CMS
	// (lan dong bo ho so dau tien tai lai theo danh sach nay).
	ListCMSUserIDs(ctx context.Context) ([]int64, error)
}

type ConversationRepository interface {
	Create(ctx context.Context, c *domain.Conversation) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Conversation, error)
	// GetMeta tra nhom KHONG kem thanh vien (nhe — dung tren duong gui tin).
	GetMeta(ctx context.Context, id uuid.UUID) (*domain.Conversation, error)
	GetByDirectKey(ctx context.Context, key string) (*domain.Conversation, error)
	ListByUser(ctx context.Context, userID uuid.UUID, f ConversationFilter, p paging.Page) ([]domain.Conversation, error)
	UpdateLastMessage(ctx context.Context, convID, msgID uuid.UUID, at time.Time) error

	AddMembers(ctx context.Context, members []domain.ConversationMember) error
	RemoveMember(ctx context.Context, convID, userID uuid.UUID) error
	GetMember(ctx context.Context, convID, userID uuid.UUID) (*domain.ConversationMember, error)
	ListMembers(ctx context.Context, convID uuid.UUID) ([]domain.ConversationMember, error)
	ListMemberUserIDs(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error)

	MarkRead(ctx context.Context, convID, userID, msgID uuid.UUID, at time.Time) error
	// SetPinned ghim (at != nil) / bo ghim (at == nil) nhom cho RIENG userID.
	SetPinned(ctx context.Context, convID, userID uuid.UUID, at *time.Time) error
	UnreadCounts(ctx context.Context, userID uuid.UUID, convIDs []uuid.UUID) (map[uuid.UUID]int64, error)
	// MentionCounts dem tin CHUA DOC nhac toi user (mentions chua mentionKey) cho tung
	// hoi thoai — cung dieu kien "chua doc" voi UnreadCounts.
	MentionCounts(ctx context.Context, userID uuid.UUID, mentionKey string, convIDs []uuid.UUID) (map[uuid.UUID]int64, error)

	// ListPeerUserIDs: nhung nguoi dung chung it nhat 1 hoi thoai voi user nay
	// (dung de phat su kien presence dung pham vi).
	ListPeerUserIDs(ctx context.Context, userID uuid.UUID) ([]uuid.UUID, error)

	// ---- nhom do CMS quan ly ----
	// ListAll liet ke moi hoi thoai (dung cho Bot API tra cuu / doi chieu dong bo).
	ListAll(ctx context.Context, f ConversationFilter, p paging.Page) ([]domain.Conversation, error)
	GetByExternalID(ctx context.Context, externalID string) (*domain.Conversation, error)
	// UpsertFromCMS tao/cap nhat nhom theo chatId cua CMS (khong doi id noi bo neu da co).
	//
	// changed = ten hoac cms_info CO doi, hoac nhom vua duoc tao. `synced_at`
	// doi moi chu ky dong bo nen khong tinh la doi — neu tinh thi vong poll 60s
	// se bao "co thay doi" mai mai.
	UpsertFromCMS(ctx context.Context, c *domain.Conversation) (conv *domain.Conversation, changed bool, err error)
	// ReplaceMembers dat danh sach thanh vien = dung danh sach CMS tra
	// (ai khong con trong danh sach thi bi danh dau left_at).
	ReplaceMembers(ctx context.Context, convID uuid.UUID, members []domain.ConversationMember) error
}

// MessageLocation: tin nhan nam o nhom nao, thread nao (khong tai noi dung).
type MessageLocation struct {
	ConversationID uuid.UUID
	GroupID        string     // chatId cua CMS; rong neu nhom tao trong Chat
	ThreadID       *uuid.UUID // nil = tin o timeline nhom
}

type MessageRepository interface {
	Create(ctx context.Context, m *domain.Message) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Message, error)
	// ListByIDs tra nhieu tin cung luc (kem Sender, Attachments); id khong ton tai thi vang mat.
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.Message, error)
	// ListSidebar: nhom + thread tron chung, moi hoat dong nhat truoc, cursor = (LastActivityAt, ID).
	ListSidebar(ctx context.Context, f SidebarFilter, p paging.Page) ([]SidebarRow, error)
	// ListPinnedSidebar: cac NHOM nguoi nay da ghim, ghim gan nhat truoc (khong phan trang).
	ListPinnedSidebar(ctx context.Context, userID uuid.UUID) ([]SidebarRow, error)
	// LastRootMessages: tin GOC moi nhat cua tung nhom (dong xem truoc cua dong nhom).
	LastRootMessages(ctx context.Context, convIDs []uuid.UUID) (map[uuid.UUID]domain.Message, error)
	// LastInThreads: tin moi nhat cua tung thread (de hien dong xem truoc tren sidebar).
	LastInThreads(ctx context.Context, rootIDs []uuid.UUID) (map[uuid.UUID]domain.Message, error)
	// GetCardByCode tra card MOI NHAT co card.header.code = code (vd "WEB-20261123-0001").
	GetCardByCode(ctx context.Context, code string) (*domain.Message, error)
	// Locations tra vi tri cua nhieu tin cung luc (tranh N+1); id khong ton tai thi vang mat.
	Locations(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]MessageLocation, error)
	GetByClientMsgID(ctx context.Context, convID uuid.UUID, clientMsgID string) (*domain.Message, error)
	List(ctx context.Context, f MessageFilter, p paging.Page) ([]domain.Message, error)
	Search(ctx context.Context, f SearchFilter, p paging.Page) ([]domain.Message, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error

	UpsertReceipts(ctx context.Context, receipts []domain.MessageReceipt) error
	ListReceipts(ctx context.Context, messageID uuid.UUID) ([]domain.MessageReceipt, error)

	// SetPinned ghim / bo ghim tin nhan.
	SetPinned(ctx context.Context, messageID uuid.UUID, pinned bool, by *uuid.UUID) error
	// ListPinned liet ke tin da ghim cua mot hoi thoai (moi ghim truoc).
	ListPinned(ctx context.Context, conversationID uuid.UUID, limit int) ([]domain.Message, error)
	// GetAttachment tra 1 tep dinh kem kem tin nhan chua no.
	GetAttachment(ctx context.Context, id uuid.UUID) (*domain.Attachment, *domain.Message, error)

	// ListReplies liet ke phan hoi cua mot tin nhan theo thu tu CU -> MOI
	// (doc thread tu tren xuong, nguoc voi timeline chat).
	ListReplies(ctx context.Context, messageID uuid.UUID, mode ReplyMode, p paging.Page) ([]domain.Message, error)
	// CountReplies dem phan hoi cho nhieu tin nhan cung luc (tranh N+1).
	CountReplies(ctx context.Context, messageIDs []uuid.UUID, mode ReplyMode) (map[uuid.UUID]int64, error)

	// UpdateCard thay card khi version moi LON HON version dang luu.
	// applied=false nghia la CMS gui version cu/bang -> bo qua (ignored).
	UpdateCard(ctx context.Context, messageID uuid.UUID, card domain.JSONMap, version int) (applied bool, err error)
}

type BotRepository interface {
	ListBots(ctx context.Context) ([]domain.Bot, error)
	GetBotBySlug(ctx context.Context, slug string) (*domain.Bot, error)
	ListTemplates(ctx context.Context, botID uuid.UUID) ([]domain.BotTemplate, error)
	GetTemplate(ctx context.Context, botID uuid.UUID, code string) (*domain.BotTemplate, error)

	CreateSubmission(ctx context.Context, s *domain.BotSubmission) error
	UpdateSubmissionStatus(ctx context.Context, id uuid.UUID, status domain.SubmissionStatus, errMsg string) error
	ListSubmissions(ctx context.Context, f SubmissionFilter, p paging.Page) ([]domain.BotSubmission, error)
}

// NotificationRepository: thong bao do CMS day sang.
type NotificationRepository interface {
	Create(ctx context.Context, n *domain.Notification) error
	ListByUser(ctx context.Context, userID uuid.UUID, f NotificationFilter, p paging.Page) ([]domain.Notification, error)
	MarkRead(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, all bool) (int64, error)
	CountUnread(ctx context.Context, userID uuid.UUID) (int64, error)
}

// IdempotencyRepository luu ket qua Bot API theo Idempotency-Key (CMS retry cung key).
type IdempotencyRepository interface {
	Get(ctx context.Context, key string) (*domain.IdempotencyRecord, error)
	Save(ctx context.Context, rec *domain.IdempotencyRecord) error
}

// OutboxRepository: event bat dong bo gui CMS (message.created, user.updated).
type OutboxRepository interface {
	Enqueue(ctx context.Context, e *domain.OutboxEvent) error
	PickPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkSent(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, nextRetry time.Time, giveUp bool) error
}

// TxManager cho phep nhieu repository tham gia chung 1 transaction.
// Transaction duoc truyen qua context nen service khong phai biet *gorm.DB.
type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// Store gom tat ca repository => inject 1 chuoi vao service.
type Store interface {
	Users() UserRepository
	Conversations() ConversationRepository
	Messages() MessageRepository
	Bots() BotRepository
	Notifications() NotificationRepository
	Idempotency() IdempotencyRepository
	Outbox() OutboxRepository
	Tx() TxManager
}
