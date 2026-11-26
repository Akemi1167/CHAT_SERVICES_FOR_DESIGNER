package cmssync

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/realtime"
)

// Dong bo ho so CMS -> Chat (ho ten, username, avatar, trang thai, vai tro).
//
// CMS khong day sang: Chat keo GET /workspace/users?updatedSince=<moc>&limit=200.
//   - Lan dau (chua co moc): tai lai theo cms_user_id cua moi actor dang co, roi
//     dat moc = luc bat dau (lui lai profileSkew de khong lot thay doi do lech dong ho).
//   - Cac lan sau: keo tu moc, hasMore thi goi tiep ngay; luu nextSince.
//
// Moc chi giu trong bo nho: khoi dong lai thi tai lai toan bo (ghi de, goi lap khong sao).
// Chi CAP NHAT actor da co trong Chat — khong tao user moi cho ca CMS.

const (
	profilePageLimit = 200
	// profileMaxPages chan vong lap vo han neu CMS tra hasMore sai.
	profileMaxPages = 50
	profileSkew     = time.Minute
)

// RunProfileSync keo ho so doi moi interval cho toi khi ctx dung.
func (s *Service) RunProfileSync(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if n, err := s.SyncProfiles(ctx); err != nil {
		s.log.Error("dong bo ho so CMS that bai (se thu lai)", "err", err)
	} else {
		s.log.Info("dong bo ho so CMS", "updated", n)
	}

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if n, err := s.SyncProfiles(ctx); err != nil {
				s.log.Error("dong bo ho so CMS that bai", "err", err)
			} else if n > 0 {
				s.log.Info("dong bo ho so CMS", "updated", n)
			}
		}
	}
}

// SyncProfiles chay mot luot dong bo, tra so actor co ho so doi.
func (s *Service) SyncProfiles(ctx context.Context) (int, error) {
	s.profileMu.Lock()
	defer s.profileMu.Unlock()

	if s.profileSince == "" {
		return s.fullProfileRefresh(ctx)
	}

	changed := 0
	for page := 0; page < profileMaxPages; page++ {
		res, err := s.client.UsersUpdatedSince(ctx, s.profileSince, profilePageLimit)
		if err != nil {
			return changed, err
		}
		n, err := s.applyProfiles(ctx, res.Users)
		changed += n
		if err != nil {
			return changed, err
		}
		if res.NextSince != "" {
			s.profileSince = res.NextSince
		}
		if !res.HasMore || res.NextSince == "" {
			break
		}
	}
	return changed, nil
}

// fullProfileRefresh: lan dau — tai lai theo id cac actor dang co, roi dat moc.
func (s *Service) fullProfileRefresh(ctx context.Context) (int, error) {
	since := time.Now().UTC().Add(-profileSkew).Format(time.RFC3339)

	ids, err := s.store.Users().ListCMSUserIDs(ctx)
	if err != nil {
		return 0, err
	}
	changed := 0
	for start := 0; start < len(ids); start += profilePageLimit {
		end := min(start+profilePageLimit, len(ids))
		users, err := s.client.Users(ctx, ids[start:end], nil)
		if err != nil {
			return changed, err
		}
		n, err := s.applyProfiles(ctx, users)
		changed += n
		if err != nil {
			return changed, err
		}
	}
	s.profileSince = since
	return changed, nil
}

// applyProfiles ghi de ho so cho actor DA CO, roi bao realtime `user` cho nhung
// nguoi chung nhom (FE cap nhat chip @, ten thanh vien, ten tren card cu).
func (s *Service) applyProfiles(ctx context.Context, profiles []cms.User) (int, error) {
	if len(profiles) == 0 {
		return 0, nil
	}
	ids := make([]int64, 0, len(profiles))
	for _, p := range profiles {
		if p.CMSUserID > 0 {
			ids = append(ids, p.CMSUserID)
		}
	}
	existing, err := s.store.Users().ListByCMSUserIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	known := make(map[int64]domain.User, len(existing))
	for _, u := range existing {
		if u.CMSUserID != nil {
			known[*u.CMSUserID] = u
		}
	}

	changed := 0
	for _, next := range s.client.ProfilesToDomain(profiles) {
		if next.CMSUserID == nil {
			continue
		}
		before, ok := known[*next.CMSUserID]
		if !ok {
			continue // chua tung xuat hien trong Chat: tao khi can (resolve / dang nhap)
		}
		next.ActorID = ActorIDFor(next.CMSUserID, next.OpenID)
		after, err := s.store.Users().UpsertFromCMS(ctx, &next)
		if err != nil {
			s.log.Error("cap nhat ho so that bai", "cms_user_id", *next.CMSUserID, "err", err)
			continue
		}
		if !profileChanged(&before, after) {
			continue
		}
		changed++
		s.publishUser(ctx, after)
	}
	return changed, nil
}

// profileChanged: chi bao realtime khi co truong FE hien thi doi.
func profileChanged(a, b *domain.User) bool {
	if a.DisplayName != b.DisplayName || a.Username != b.Username || a.AvatarURL != b.AvatarURL ||
		a.Status != b.Status || a.Role != b.Role || a.Email != b.Email || a.IsDesigner != b.IsDesigner {
		return true
	}
	return a.DisplayRole() != b.DisplayRole() || !sameProfile(a.CMSProfile, b.CMSProfile)
}

func sameProfile(a, b domain.JSONMap) bool {
	for _, k := range []string{"webTeams", "assistantOf", "botAdminOf"} {
		if !jsonEqual(a[k], b[k]) {
			return false
		}
	}
	return true
}

func (s *Service) publishUser(ctx context.Context, u *domain.User) {
	if s.hub == nil {
		return
	}
	peers, err := s.store.Conversations().ListPeerUserIDs(ctx, u.ID)
	if err != nil || len(peers) == 0 {
		peers = []uuid.UUID{u.ID}
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventUserUpdated, "", peers, map[string]any{
		"user_id":    u.ID,
		"actor_id":   u.ActorID,
		"name":       u.DisplayName,
		"username":   u.Username,
		"avatar_url": u.AvatarURL,
		"role":       u.DisplayRole(),
		"status":     u.Status,
	}))
}

func jsonEqual(a, b any) bool {
	ja, errA := json.Marshal(a)
	jb, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(ja, jb)
}
