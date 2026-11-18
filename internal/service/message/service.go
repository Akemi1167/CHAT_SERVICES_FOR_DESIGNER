// Package message xu ly gui/doc tin nhan, thread, trang thai da doc va dieu phoi bot.
package message

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/bot"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
)

type Service struct {
	store repository.Store
	convs *conversation.Service
	bots  *bot.Service
	hub   *realtime.Hub
	log   *slog.Logger

	// hooks chay sau khi tin nhan cua nguoi dung da luu (vd day event sang CMS).
	hooks []func(ctx context.Context, msg *domain.Message)
}

// AddMessageHook dang ky viec can lam sau khi tao tin nhan (khong chan request).
func (s *Service) AddMessageHook(fn func(ctx context.Context, msg *domain.Message)) {
	if fn != nil {
		s.hooks = append(s.hooks, fn)
	}
}

func New(store repository.Store, convs *conversation.Service, bots *bot.Service, hub *realtime.Hub) *Service {
	return &Service{
		store: store, convs: convs, bots: bots, hub: hub,
		log: slog.Default().With("component", "service.message"),
	}
}

type SendInput struct {
	ActorID        uuid.UUID
	ConversationID uuid.UUID
	Type           domain.MessageType
	Content        string
	ThreadID       *uuid.UUID
	ReplyToID      *uuid.UUID
	Metadata       domain.JSONMap
	Attachments    []domain.Attachment
	// ClientMsgID: UUID do FE sinh. Gui lai cung ID => tra ve tin nhan cu (idempotent).
	ClientMsgID string

	// Duong "gui mau" tu form FE (thay vi go "@bot ..." trong noi dung).
	BotSlug         string
	TemplateCode    string
	TemplatePayload domain.JSONMap
}

type SendResult struct {
	Message    *domain.Message
	BotReply   *domain.Message
	Submission *domain.BotSubmission
	// Duplicated = true khi tra ve tin nhan cu theo ClientMsgID.
	Duplicated bool
}

// Send la diem vao duy nhat de tao tin nhan (REST, WebSocket va gRPC dung chung).
func (s *Service) Send(ctx context.Context, in SendInput) (*SendResult, error) {
	if _, err := s.convs.EnsureMember(ctx, in.ConversationID, in.ActorID); err != nil {
		return nil, err
	}

	// 1) Idempotency: FE retry sau khi mat ket noi WS.
	// Luu y: tin trung se KHONG duoc tao lai va KHONG broadcast — nguoi gui chi nhan
	// lai tin cu kem Duplicated=true. Khi test tay nho doi client_msg_id moi.
	if in.ClientMsgID != "" {
		if existing, err := s.store.Messages().GetByClientMsgID(ctx, in.ConversationID, in.ClientMsgID); err == nil {
			s.log.Info("bo qua tin nhan trung client_msg_id (khong broadcast)",
				"conversation_id", in.ConversationID, "client_msg_id", in.ClientMsgID,
				"message_id", existing.ID, "sender_id", in.ActorID)
			return &SendResult{Message: existing, Duplicated: true}, nil
		} else if !isNotFound(err) {
			return nil, err
		}
	}

	msgType := in.Type
	if msgType == "" {
		msgType = domain.MessageText
	}
	if !msgType.Valid() {
		return nil, apperr.Invalid("invalid_message_type", "type tin nhan khong hop le")
	}
	if msgType == domain.MessageSystem || msgType == domain.MessageBotReply {
		return nil, apperr.Forbidden("reserved_message_type", "type nay chi he thong/bot duoc dung")
	}

	content := strings.TrimSpace(in.Content)
	if content == "" && len(in.Attachments) == 0 && in.TemplateCode == "" {
		return nil, apperr.Invalid("empty_message", "Tin nhan phai co noi dung hoac tep")
	}

	// 2) Xac dinh co goi bot khong.
	botSlug := strings.TrimPrefix(strings.TrimSpace(in.BotSlug), "@")
	if botSlug == "" {
		if slug, ok := bot.Detect(content); ok {
			botSlug = slug
		}
	}
	if botSlug != "" {
		msgType = domain.MessageBotCommand
	}

	// 3) Kiem tra thread / reply thuoc cung hoi thoai.
	if err := s.validateRefs(ctx, in.ConversationID, in.ThreadID, in.ReplyToID); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	msg := &domain.Message{
		ID:             uuid.New(),
		ConversationID: in.ConversationID,
		SenderID:       &in.ActorID,
		SenderType:     domain.SenderUser,
		Type:           msgType,
		Content:        content,
		ThreadID:       in.ThreadID,
		ReplyToID:      in.ReplyToID,
		Metadata:       in.Metadata,
		ClientMsgID:    in.ClientMsgID,
		Attachments:    in.Attachments,
		CreatedAt:      now,
	}

	// 4) Ghi DB trong transaction: message + con tro last_message cua hoi thoai.
	err := s.store.Tx().Do(ctx, func(ctx context.Context) error {
		if err := s.store.Messages().Create(ctx, msg); err != nil {
			return err
		}
		return s.store.Conversations().UpdateLastMessage(ctx, in.ConversationID, msg.ID, msg.CreatedAt)
	})
	if err != nil {
		// Race idempotency: 2 request cung client_msg_id vao cung luc.
		if apperr.From(err).Kind == apperr.KindConflict && in.ClientMsgID != "" {
			if existing, e := s.store.Messages().GetByClientMsgID(ctx, in.ConversationID, in.ClientMsgID); e == nil {
				return &SendResult{Message: existing, Duplicated: true}, nil
			}
		}
		return nil, err
	}

	saved, err := s.store.Messages().GetByID(ctx, msg.ID)
	if err != nil {
		return nil, err
	}

	recipients, err := s.convs.MemberUserIDs(ctx, in.ConversationID)
	if err != nil {
		return nil, err
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventMessageNew, in.ConversationID.String(), recipients, saved))

	for _, hook := range s.hooks {
		hook(ctx, saved)
	}

	result := &SendResult{Message: saved}

	// 5) Chay bot NGOAI transaction: bot loi thi tin nhan nguoi dung van con.
	if botSlug != "" {
		reply, sub := s.runBot(ctx, in, saved, botSlug, recipients)
		result.BotReply = reply
		result.Submission = sub
	}
	return result, nil
}

// runBot dispatch bot roi tao tin nhan tra loi cua bot trong cung hoi thoai/thread.
func (s *Service) runBot(
	ctx context.Context,
	in SendInput,
	userMsg *domain.Message,
	slug string,
	recipients []uuid.UUID,
) (*domain.Message, *domain.BotSubmission) {
	threadID := userMsg.ThreadID
	res, err := s.bots.Dispatch(ctx, bot.DispatchInput{
		Content:        userMsg.Content,
		Slug:           slug,
		TemplateCode:   in.TemplateCode,
		Payload:        in.TemplatePayload,
		UserID:         in.ActorID,
		ConversationID: in.ConversationID,
		MessageID:      &userMsg.ID,
		ThreadID:       threadID,
	})

	replyText := ""
	meta := domain.JSONMap{"bot_slug": slug, "in_reply_to": userMsg.ID.String()}
	var sub *domain.BotSubmission

	switch {
	case err != nil:
		e := apperr.From(err)
		if e.Kind == apperr.KindInternal {
			s.log.Error("bot dispatch failed", "bot", slug, "err", e.Error())
		}
		replyText = "Khong xu ly duoc yeu cau: " + e.Message
		meta["error_code"] = e.Code
		if e.Details != nil {
			meta["error_details"] = e.Details
		}
	case res == nil:
		return nil, nil
	case res.Silent:
		// Bot khong tra loi trong chat (vd task service da dang card roi),
		// nhung van thong bao mau da luu.
		if res.Submission != nil {
			s.hub.Publish(ctx, realtime.NewEvent(
				realtime.EventBotSubmissionStored, in.ConversationID.String(), recipients, res.Submission,
			))
		}
		return nil, res.Submission
	default:
		replyText = res.ReplyText
		sub = res.Submission
		for k, v := range res.ReplyMeta {
			meta[k] = v
		}
	}

	reply := &domain.Message{
		ID:             uuid.New(),
		ConversationID: in.ConversationID,
		SenderID:       nil,
		SenderType:     domain.SenderBot,
		Type:           domain.MessageBotReply,
		Content:        replyText,
		ThreadID:       threadID,
		ReplyToID:      &userMsg.ID,
		Metadata:       meta,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.Messages().Create(ctx, reply); err != nil {
		s.log.Error("tao tin nhan bot that bai", "err", err)
		return nil, sub
	}
	_ = s.store.Conversations().UpdateLastMessage(ctx, in.ConversationID, reply.ID, reply.CreatedAt)

	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventMessageNew, in.ConversationID.String(), recipients, reply))
	if sub != nil {
		s.hub.Publish(ctx, realtime.NewEvent(
			realtime.EventBotSubmissionStored, in.ConversationID.String(), recipients, sub,
		))
	}
	return reply, sub
}

type ListInput struct {
	ActorID        uuid.UUID
	ConversationID uuid.UUID
	ThreadID       *uuid.UUID
	// OnlyRoot: chi lay tin nhan ngoai thread (dung cho khung chat chinh neu muon).
	OnlyRoot bool
}

func (s *Service) List(ctx context.Context, in ListInput, p paging.Page) ([]domain.Message, paging.PageInfo, error) {
	if _, err := s.convs.EnsureMember(ctx, in.ConversationID, in.ActorID); err != nil {
		return nil, paging.PageInfo{}, err
	}
	rows, err := s.store.Messages().List(ctx, repository.MessageFilter{
		ConversationID: in.ConversationID,
		ThreadID:       in.ThreadID,
		OnlyRoot:       in.OnlyRoot,
	}, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, msgKey)
	if err := s.attachReplyCounts(ctx, items); err != nil {
		return nil, paging.PageInfo{}, err
	}
	return items, info, nil
}

// attachReplyCounts dem phan hoi cho ca trang bang 2 query (tranh N+1).
func (s *Service) attachReplyCounts(ctx context.Context, items []domain.Message) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(items))
	roots := make([]uuid.UUID, 0, len(items))
	for _, m := range items {
		ids = append(ids, m.ID)
		if m.ThreadID == nil {
			roots = append(roots, m.ID)
		}
	}

	direct, err := s.store.Messages().CountReplies(ctx, ids, repository.ReplyDirect)
	if err != nil {
		return err
	}
	thread := map[uuid.UUID]int64{}
	if len(roots) > 0 {
		if thread, err = s.store.Messages().CountReplies(ctx, roots, repository.ReplyThread); err != nil {
			return err
		}
	}
	for i := range items {
		items[i].ReplyCount = direct[items[i].ID]
		items[i].ThreadCount = thread[items[i].ID]
	}
	return nil
}

// ---------------- phan hoi cua mot tin nhan ----------------

type RepliesInput struct {
	ActorID   uuid.UUID
	MessageID uuid.UUID
	// Mode: direct (mac dinh) = tin tra loi truc tiep; thread = ca thread tin nay mo.
	Mode repository.ReplyMode
}

// RepliesResult gom tin goc + danh sach phan hoi + so dem.
type RepliesResult struct {
	Root        *domain.Message  `json:"root"`
	Replies     []domain.Message `json:"replies"`
	PageInfo    paging.PageInfo  `json:"page_info"`
	DirectCount int64            `json:"reply_count"`
	ThreadCount int64            `json:"thread_count"`
}

// Replies tra cac tin nhan tra loi mot tin nhan (theo thu tu cu -> moi).
func (s *Service) Replies(ctx context.Context, in RepliesInput, p paging.Page) (*RepliesResult, error) {
	root, err := s.store.Messages().GetByID(ctx, in.MessageID)
	if err != nil {
		return nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, root.ConversationID, in.ActorID); err != nil {
		return nil, err
	}

	mode := in.Mode
	if mode != repository.ReplyThread {
		mode = repository.ReplyDirect
	}

	rows, err := s.store.Messages().ListReplies(ctx, in.MessageID, mode, p)
	if err != nil {
		return nil, err
	}
	items, info := paging.Build(rows, p, msgKey)
	if err := s.attachReplyCounts(ctx, items); err != nil {
		return nil, err
	}

	counts, err := s.store.Messages().CountReplies(ctx, []uuid.UUID{in.MessageID}, repository.ReplyDirect)
	if err != nil {
		return nil, err
	}
	threadCounts, err := s.store.Messages().CountReplies(ctx, []uuid.UUID{in.MessageID}, repository.ReplyThread)
	if err != nil {
		return nil, err
	}
	root.ReplyCount = counts[in.MessageID]
	root.ThreadCount = threadCounts[in.MessageID]

	return &RepliesResult{
		Root:        root,
		Replies:     items,
		PageInfo:    info,
		DirectCount: root.ReplyCount,
		ThreadCount: root.ThreadCount,
	}, nil
}

type SearchInput struct {
	ActorID        uuid.UUID
	ConversationID uuid.UUID // uuid.Nil = tim trong tat ca hoi thoai cua user
	Query          string
	SenderID       *uuid.UUID
	FromDate       *time.Time
	ToDate         *time.Time
}

func (s *Service) Search(ctx context.Context, in SearchInput, p paging.Page) ([]domain.Message, paging.PageInfo, error) {
	if in.ConversationID != uuid.Nil {
		if _, err := s.convs.EnsureMember(ctx, in.ConversationID, in.ActorID); err != nil {
			return nil, paging.PageInfo{}, err
		}
	}
	rows, err := s.store.Messages().Search(ctx, repository.SearchFilter{
		ConversationID: in.ConversationID,
		UserID:         in.ActorID,
		Query:          in.Query,
		SenderID:       in.SenderID,
		FromDate:       in.FromDate,
		ToDate:         in.ToDate,
	}, p)
	if err != nil {
		return nil, paging.PageInfo{}, err
	}
	items, info := paging.Build(rows, p, msgKey)
	if err := s.attachReplyCounts(ctx, items); err != nil {
		return nil, paging.PageInfo{}, err
	}
	return items, info, nil
}

// MarkRead cap nhat moc da doc + luu receipt (de hien "danh sach nguoi da xem").
func (s *Service) MarkRead(ctx context.Context, actorID, convID, messageID uuid.UUID) (int64, error) {
	if _, err := s.convs.EnsureMember(ctx, convID, actorID); err != nil {
		return 0, err
	}
	msg, err := s.store.Messages().GetByID(ctx, messageID)
	if err != nil {
		return 0, err
	}
	if msg.ConversationID != convID {
		return 0, apperr.Invalid("message_conversation_mismatch", "Tin nhan khong thuoc hoi thoai nay")
	}

	now := time.Now().UTC()
	err = s.store.Tx().Do(ctx, func(ctx context.Context) error {
		if err := s.store.Conversations().MarkRead(ctx, convID, actorID, messageID, msg.CreatedAt); err != nil {
			return err
		}
		return s.store.Messages().UpsertReceipts(ctx, []domain.MessageReceipt{
			{MessageID: messageID, UserID: actorID, SeenAt: now},
		})
	})
	if err != nil {
		return 0, err
	}

	recipients, err := s.convs.MemberUserIDs(ctx, convID)
	if err != nil {
		return 0, err
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventMessageRead, convID.String(), recipients, map[string]any{
		"conversation_id": convID,
		"message_id":      messageID,
		"user_id":         actorID,
		"seen_at":         now,
	}))

	counts, err := s.store.Conversations().UnreadCounts(ctx, actorID, []uuid.UUID{convID})
	if err != nil {
		return 0, err
	}
	return counts[convID], nil
}

// Readers tra danh sach user da xem 1 tin nhan.
func (s *Service) Readers(ctx context.Context, actorID, messageID uuid.UUID) ([]domain.MessageReceipt, error) {
	msg, err := s.store.Messages().GetByID(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if _, err := s.convs.EnsureMember(ctx, msg.ConversationID, actorID); err != nil {
		return nil, err
	}
	return s.store.Messages().ListReceipts(ctx, messageID)
}

// Typing phat su kien "dang nhap..." (khong luu DB).
func (s *Service) Typing(ctx context.Context, actorID, convID uuid.UUID, typing bool) error {
	if _, err := s.convs.EnsureMember(ctx, convID, actorID); err != nil {
		return err
	}
	recipients, err := s.convs.MemberUserIDs(ctx, convID)
	if err != nil {
		return err
	}
	ev := realtime.NewEvent(realtime.EventTyping, convID.String(), recipients, map[string]any{
		"conversation_id": convID,
		"user_id":         actorID,
		"typing":          typing,
	})
	ev.SkipUserID = &actorID
	s.hub.Publish(ctx, ev)
	return nil
}

// Delete: xoa mem, chi nguoi gui hoac admin/owner cua nhom.
func (s *Service) Delete(ctx context.Context, actorID, messageID uuid.UUID) error {
	msg, err := s.store.Messages().GetByID(ctx, messageID)
	if err != nil {
		return err
	}
	member, err := s.convs.EnsureMember(ctx, msg.ConversationID, actorID)
	if err != nil {
		return err
	}
	isOwner := msg.SenderID != nil && *msg.SenderID == actorID
	isAdmin := member.Role == domain.MemberOwner || member.Role == domain.MemberAdmin
	if !isOwner && !isAdmin {
		return apperr.Forbidden("cannot_delete_message", "Khong co quyen xoa tin nhan nay")
	}
	if err := s.store.Messages().SoftDelete(ctx, messageID); err != nil {
		return err
	}
	recipients, err := s.convs.MemberUserIDs(ctx, msg.ConversationID)
	if err != nil {
		return err
	}
	s.hub.Publish(ctx, realtime.NewEvent(
		realtime.EventMessageDeleted, msg.ConversationID.String(), recipients,
		map[string]any{"conversation_id": msg.ConversationID, "message_id": messageID},
	))
	return nil
}

func (s *Service) validateRefs(ctx context.Context, convID uuid.UUID, threadID, replyToID *uuid.UUID) error {
	check := func(id *uuid.UUID, code string) error {
		if id == nil {
			return nil
		}
		m, err := s.store.Messages().GetByID(ctx, *id)
		if err != nil {
			return err
		}
		if m.ConversationID != convID {
			return apperr.Invalid(code, "Tin nhan tham chieu khong thuoc hoi thoai nay")
		}
		return nil
	}
	if err := check(threadID, "invalid_thread_id"); err != nil {
		return err
	}
	return check(replyToID, "invalid_reply_to_id")
}

func msgKey(m domain.Message) (time.Time, string) { return m.CreatedAt, m.ID.String() }

func isNotFound(err error) bool {
	var e *apperr.Error
	if errors.As(err, &e) {
		return e.Kind == apperr.KindNotFound
	}
	return false
}

// ---------------- tin nhan he thong ----------------

// SystemMessageInput la input de cac service khac (vd task) dang tin nhan he thong
// vao hoi thoai/thread. Day la CUA DUY NHAT ghi vao bang messages.
type SystemMessageInput struct {
	ConversationID uuid.UUID
	ThreadID       *uuid.UUID
	Content        string
	Metadata       domain.JSONMap
	// ActorID != nil: hien ten nguoi thao tac nhung van la tin he thong.
	ActorID *uuid.UUID
}

// PostSystem tao tin nhan he thong (sender_type = system) va broadcast realtime.
// Khong kiem tra quyen: nguoi goi (service noi bo) da kiem tra truoc do.
func (s *Service) PostSystem(ctx context.Context, in SystemMessageInput) (*domain.Message, error) {
	meta := in.Metadata
	if meta == nil {
		meta = domain.JSONMap{}
	}
	if in.ActorID != nil {
		meta["actor_id"] = in.ActorID.String()
	}

	msg := &domain.Message{
		ID:             uuid.New(),
		ConversationID: in.ConversationID,
		SenderID:       nil,
		SenderType:     domain.SenderSystem,
		Type:           domain.MessageSystem,
		Content:        in.Content,
		ThreadID:       in.ThreadID,
		Metadata:       meta,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.Messages().Create(ctx, msg); err != nil {
		return nil, err
	}
	if err := s.store.Conversations().UpdateLastMessage(ctx, in.ConversationID, msg.ID, msg.CreatedAt); err != nil {
		s.log.Warn("cap nhat last_message that bai", "err", err)
	}

	recipients, err := s.convs.MemberUserIDs(ctx, in.ConversationID)
	if err != nil {
		return nil, err
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventMessageNew, in.ConversationID.String(), recipients, msg))
	return msg, nil
}

// BotMessageInput: tin nhan do bot CMS gui qua Bot API (text hoac card).
type BotMessageInput struct {
	ConversationID uuid.UUID
	ThreadID       *uuid.UUID
	Content        string
	Card           domain.JSONMap
	CardVersion    int
	// BotSender = {"id":1,"name":"Demo Designer Bot"} de UI hien dung nguoi gui.
	BotSender domain.JSONMap
	Metadata  domain.JSONMap
}

// PostBot tao tin nhan cua bot. KHONG tu broadcast: card co visibleTo nen
// nguoi goi (Bot API) phat qua card.Service.Broadcast de loc rieng cho tung nguoi.
func (s *Service) PostBot(ctx context.Context, in BotMessageInput) (*domain.Message, error) {
	msgType := domain.MessageBotReply
	if in.Card != nil {
		msgType = domain.MessageBotCommand
	}
	cardVersion := in.CardVersion
	if in.Card != nil && cardVersion <= 0 {
		cardVersion = 1
	}

	msg := &domain.Message{
		ID:             uuid.New(),
		ConversationID: in.ConversationID,
		SenderID:       nil,
		SenderType:     domain.SenderBot,
		Type:           msgType,
		Content:        in.Content,
		ThreadID:       in.ThreadID,
		Card:           in.Card,
		CardVersion:    cardVersion,
		BotSender:      in.BotSender,
		Metadata:       in.Metadata,
		CreatedAt:      time.Now().UTC(),
	}
	if err := s.store.Messages().Create(ctx, msg); err != nil {
		return nil, err
	}
	if err := s.store.Conversations().UpdateLastMessage(ctx, in.ConversationID, msg.ID, msg.CreatedAt); err != nil {
		s.log.Warn("cap nhat last_message that bai", "err", err)
	}
	return msg, nil
}
