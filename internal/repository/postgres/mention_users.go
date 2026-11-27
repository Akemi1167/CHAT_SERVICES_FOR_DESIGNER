package postgres

import (
	"context"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// attachMentionUsers dien Message.MentionUsers (ho so nguoi duoc nhac, theo dung thu tu
// Message.Mentions) cho ca trang bang MOT query. Trang khong co tin nao nhac ten thi
// khong query. Doc moi lan nen ten / username / vai tro luon la ban moi nhat.
func (r *MessageRepo) attachMentionUsers(ctx context.Context, msgs []domain.Message) error {
	var actorIDs []string
	var userIDs []uuid.UUID
	seen := map[string]bool{}
	for i := range msgs {
		for _, k := range msgs[i].Mentions {
			if seen[k] {
				continue
			}
			seen[k] = true
			if id, err := uuid.Parse(k); err == nil {
				userIDs = append(userIDs, id) // user chua co actorId: khoa la UUID
			} else {
				actorIDs = append(actorIDs, k)
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}

	q := r.db(ctx).Model(&domain.User{})
	switch {
	case len(actorIDs) > 0 && len(userIDs) > 0:
		q = q.Where("actor_id IN ? OR id IN ?", actorIDs, userIDs)
	case len(actorIDs) > 0:
		q = q.Where("actor_id IN ?", actorIDs)
	default:
		q = q.Where("id IN ?", userIDs)
	}
	var users []domain.User
	if err := q.Find(&users).Error; err != nil {
		return wrap(err, "message.attachMentionUsers")
	}
	byKey := make(map[string]domain.User, len(users)*2)
	for _, u := range users {
		byKey[u.ID.String()] = u
		if u.ActorID != "" {
			byKey[u.ActorID] = u
		}
	}

	for i := range msgs {
		if len(msgs[i].Mentions) == 0 {
			continue
		}
		out := make([]domain.User, 0, len(msgs[i].Mentions))
		for _, k := range msgs[i].Mentions {
			if u, ok := byKey[k]; ok {
				out = append(out, u)
			} else {
				out = append(out, domain.User{ActorID: k}) // user da bi xoa khoi Chat: giu khoa
			}
		}
		msgs[i].MentionUsers = out
	}
	return nil
}

// attachMentionUsersOne: ban cho 1 tin.
func (r *MessageRepo) attachMentionUsersOne(ctx context.Context, m *domain.Message) error {
	if m == nil || len(m.Mentions) == 0 {
		return nil
	}
	list := []domain.Message{*m}
	if err := r.attachMentionUsers(ctx, list); err != nil {
		return err
	}
	m.MentionUsers = list[0].MentionUsers
	return nil
}

// attachMentionUsersMap: ban cho map[key]Message (tin cuoi cua nhom / thread).
func (r *MessageRepo) attachMentionUsersMap(ctx context.Context, in map[uuid.UUID]domain.Message) error {
	keys := make([]uuid.UUID, 0, len(in))
	list := make([]domain.Message, 0, len(in))
	for k, m := range in {
		if len(m.Mentions) == 0 {
			continue
		}
		keys = append(keys, k)
		list = append(list, m)
	}
	if len(list) == 0 {
		return nil
	}
	if err := r.attachMentionUsers(ctx, list); err != nil {
		return err
	}
	for i, k := range keys {
		in[k] = list[i]
	}
	return nil
}
