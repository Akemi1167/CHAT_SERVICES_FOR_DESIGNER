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

// TaskFilter: moi field nil/rong = khong filter.
// MemberUserID luon duoc service set = nguoi goi, de chi thay task trong hoi thoai minh tham gia.
type TaskFilter struct {
	MemberUserID       uuid.UUID
	ConversationID     *uuid.UUID
	ThreadID           *uuid.UUID
	TeamID             *uuid.UUID
	CreatedBy          *uuid.UUID
	AssigneeID         *uuid.UUID
	Statuses           []domain.TaskStatus
	AssignmentStatuses []domain.AssignmentStatus
	Query              string
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
}

type ConversationRepository interface {
	Create(ctx context.Context, c *domain.Conversation) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Conversation, error)
	GetByDirectKey(ctx context.Context, key string) (*domain.Conversation, error)
	ListByUser(ctx context.Context, userID uuid.UUID, f ConversationFilter, p paging.Page) ([]domain.Conversation, error)
	UpdateLastMessage(ctx context.Context, convID, msgID uuid.UUID, at time.Time) error

	AddMembers(ctx context.Context, members []domain.ConversationMember) error
	RemoveMember(ctx context.Context, convID, userID uuid.UUID) error
	GetMember(ctx context.Context, convID, userID uuid.UUID) (*domain.ConversationMember, error)
	ListMembers(ctx context.Context, convID uuid.UUID) ([]domain.ConversationMember, error)
	ListMemberUserIDs(ctx context.Context, convID uuid.UUID) ([]uuid.UUID, error)

	MarkRead(ctx context.Context, convID, userID, msgID uuid.UUID, at time.Time) error
	UnreadCounts(ctx context.Context, userID uuid.UUID, convIDs []uuid.UUID) (map[uuid.UUID]int64, error)

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

// TaskRepository.
//
// Quy uoc: cac ham Update* nhan map[string]any de co the set NULL va chi ghi
// dung cot can ghi (GORM Updates voi struct se bo qua gia tri zero).
type TaskRepository interface {
	NextTaskCode(ctx context.Context) (string, error)
	NextRevisionCode(ctx context.Context) (string, error)

	Create(ctx context.Context, t *domain.Task) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Task, error)
	GetByCode(ctx context.Context, code string) (*domain.Task, error)
	List(ctx context.Context, f TaskFilter, p paging.Page) ([]domain.Task, error)
	UpdateTask(ctx context.Context, id uuid.UUID, fields map[string]any) error

	CreateAssignments(ctx context.Context, items []domain.TaskAssignment) error
	GetAssignment(ctx context.Context, id uuid.UUID) (*domain.TaskAssignment, error)
	ListAssignments(ctx context.Context, taskID uuid.UUID) ([]domain.TaskAssignment, error)
	UpdateAssignment(ctx context.Context, id uuid.UUID, fields map[string]any) error
	// CountOpenAssignments dem viec dang mo cua tung user (de goi y nguoi phu trach).
	CountOpenAssignments(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]int64, error)

	CreateRevision(ctx context.Context, r *domain.TaskRevision) error
	GetRevision(ctx context.Context, id uuid.UUID) (*domain.TaskRevision, error)
	ListRevisions(ctx context.Context, taskID uuid.UUID) ([]domain.TaskRevision, error)
	UpdateRevision(ctx context.Context, id uuid.UUID, fields map[string]any) error

	CreateEvent(ctx context.Context, e *domain.TaskEvent) error
	ListEvents(ctx context.Context, taskID uuid.UUID, p paging.Page) ([]domain.TaskEvent, error)
}

type TeamRepository interface {
	Create(ctx context.Context, t *domain.Team) error
	List(ctx context.Context) ([]domain.Team, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Team, error)
	GetByCode(ctx context.Context, code string) (*domain.Team, error)
	AddMembers(ctx context.Context, members []domain.TeamMember) error
	RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error
	ListMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMember, error)
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
	Tasks() TaskRepository
	Teams() TeamRepository
	Notifications() NotificationRepository
	Idempotency() IdempotencyRepository
	Outbox() OutboxRepository
	Tx() TxManager
}
