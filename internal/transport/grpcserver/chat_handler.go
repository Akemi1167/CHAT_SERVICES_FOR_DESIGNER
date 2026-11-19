package grpcserver

import (
	"context"

	"github.com/google/uuid"

	chatv1 "github.com/abc-workspace/chat-service/gen/go/chat/v1"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/conversation"
	"github.com/abc-workspace/chat-service/internal/service/message"
)

// ChatHandler chi lam: parse request -> goi service -> map response.
// Khong co business logic o day.
type ChatHandler struct {
	chatv1.UnimplementedChatServiceServer
	convs *conversation.Service
	msgs  *message.Service
}

func NewChatHandler(convs *conversation.Service, msgs *message.Service) *ChatHandler {
	return &ChatHandler{convs: convs, msgs: msgs}
}

func (h *ChatHandler) CreateConversation(ctx context.Context, req *chatv1.CreateConversationRequest) (*chatv1.CreateConversationResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	members, err := parseUUIDs(req.GetMemberUserIds(), "member_user_ids")
	if err != nil {
		return nil, err
	}
	conv, err := h.convs.Create(ctx, conversation.CreateInput{
		ActorID:       actor,
		Type:          convTypeFrom(req.GetType()),
		Name:          req.GetName(),
		AvatarURL:     req.GetAvatarUrl(),
		MemberUserIDs: members,
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.CreateConversationResponse{Conversation: conversationTo(conv)}, nil
}

func (h *ChatHandler) GetConversation(ctx context.Context, req *chatv1.GetConversationRequest) (*chatv1.GetConversationResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	conv, err := h.convs.Get(ctx, actor, convID)
	if err != nil {
		return nil, err
	}
	return &chatv1.GetConversationResponse{Conversation: conversationTo(conv)}, nil
}

func (h *ChatHandler) ListConversations(ctx context.Context, req *chatv1.ListConversationsRequest) (*chatv1.ListConversationsResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	p := pageFrom(req.GetPage())
	// Proto chua co field filter => khong loc.
	items, info, err := h.convs.List(ctx, actor, repository.ConversationFilter{}, p)
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.Conversation, 0, len(items))
	for i := range items {
		out = append(out, conversationTo(&items[i]))
	}
	return &chatv1.ListConversationsResponse{Conversations: out, PageInfo: pageInfoTo(info)}, nil
}

func (h *ChatHandler) AddMembers(ctx context.Context, req *chatv1.AddMembersRequest) (*chatv1.AddMembersResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	userIDs, err := parseUUIDs(req.GetUserIds(), "user_ids")
	if err != nil {
		return nil, err
	}
	conv, err := h.convs.AddMembers(ctx, conversation.AddMembersInput{
		ActorID: actor, ConvID: convID, UserIDs: userIDs, Role: roleFrom(req.GetRole()),
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.AddMembersResponse{Conversation: conversationTo(conv)}, nil
}

func (h *ChatHandler) RemoveMember(ctx context.Context, req *chatv1.RemoveMemberRequest) (*chatv1.RemoveMemberResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := h.convs.RemoveMember(ctx, actor, convID, userID); err != nil {
		return nil, err
	}
	return &chatv1.RemoveMemberResponse{}, nil
}

func (h *ChatHandler) SendMessage(ctx context.Context, req *chatv1.SendMessageRequest) (*chatv1.SendMessageResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	threadID, err := parseUUIDPtr(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	replyTo, err := parseUUIDPtr(req.GetReplyToId(), "reply_to_id")
	if err != nil {
		return nil, err
	}
	meta, err := domain.ParseJSONMap(req.GetMetadataJson())
	if err != nil {
		return nil, err
	}

	res, err := h.msgs.Send(ctx, message.SendInput{
		ActorID:        actor,
		ConversationID: convID,
		Type:           msgTypeFrom(req.GetType()),
		Content:        req.GetContent(),
		ThreadID:       threadID,
		ReplyToID:      replyTo,
		Metadata:       meta,
		Attachments:    attachmentsFrom(req.GetAttachments()),
		ClientMsgID:    req.GetClientMsgId(),
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.SendMessageResponse{Message: messageTo(res.Message)}, nil
}

func (h *ChatHandler) ListMessages(ctx context.Context, req *chatv1.ListMessagesRequest) (*chatv1.ListMessagesResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	threadID, err := parseUUIDPtr(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	items, info, err := h.msgs.List(ctx, message.ListInput{
		ActorID: actor, ConversationID: convID, ThreadID: threadID,
	}, pageFrom(req.GetPage()))
	if err != nil {
		return nil, err
	}
	return &chatv1.ListMessagesResponse{Messages: messagesTo(items), PageInfo: pageInfoTo(info)}, nil
}

func (h *ChatHandler) SearchMessages(ctx context.Context, req *chatv1.SearchMessagesRequest) (*chatv1.SearchMessagesResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID := uuid.Nil
	if req.GetConversationId() != "" {
		convID, err = parseUUID(req.GetConversationId(), "conversation_id")
		if err != nil {
			return nil, err
		}
	}
	senderID, err := parseUUIDPtr(req.GetSenderId(), "sender_id")
	if err != nil {
		return nil, err
	}
	from, err := parseTimePtr(req.GetFromDate(), "from_date")
	if err != nil {
		return nil, err
	}
	to, err := parseTimePtr(req.GetToDate(), "to_date")
	if err != nil {
		return nil, err
	}
	items, info, err := h.msgs.Search(ctx, message.SearchInput{
		ActorID: actor, ConversationID: convID, Query: req.GetQuery(),
		SenderID: senderID, FromDate: from, ToDate: to,
	}, pageFrom(req.GetPage()))
	if err != nil {
		return nil, err
	}
	return &chatv1.SearchMessagesResponse{Messages: messagesTo(items), PageInfo: pageInfoTo(info)}, nil
}

func (h *ChatHandler) MarkRead(ctx context.Context, req *chatv1.MarkReadRequest) (*chatv1.MarkReadResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	msgID, err := parseUUID(req.GetMessageId(), "message_id")
	if err != nil {
		return nil, err
	}
	unread, err := h.msgs.MarkRead(ctx, actor, convID, msgID)
	if err != nil {
		return nil, err
	}
	return &chatv1.MarkReadResponse{UnreadCount: unread}, nil
}
