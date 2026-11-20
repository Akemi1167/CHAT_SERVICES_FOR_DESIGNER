// Package presence quan ly trang thai online/OFF cua nguoi dung.
//
// Hai nguon cap nhat:
//   - Chat tu biet: mo/dong WebSocket (online <-> offline)
//   - CMS bao sang: PUT /bot/v1/presence (vd lich OFF, het ca) kem ly do
//
// Su kien chi phat cho nhung nguoi CHUNG hoi thoai, khong broadcast toan he thong.
package presence

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type Service struct {
	store repository.Store
	hub   *realtime.Hub
	log   *slog.Logger

	// heartbeat: chu ky lam tuoi last_seen_at cho nguoi dang noi vao instance nay.
	heartbeat time.Duration
}

func New(store repository.Store, hub *realtime.Hub) *Service {
	return &Service{store: store, hub: hub, heartbeat: domain.DefaultPresenceHeartbeat, log: slog.Default().With("component", "service.presence")}
}

// SetOnline goi khi WebSocket mo/dong. Online thi xoa ly do OFF.
func (s *Service) SetOnline(ctx context.Context, userID uuid.UUID, online bool) error {
	status := domain.PresenceOffline
	reason := ""
	if online {
		status = domain.PresenceOnline
	} else {
		// Giu ly do OFF do CMS dat (vd dang nghi phep) khi nguoi dung chi dong tab.
		if u, err := s.store.Users().GetByID(ctx, userID); err == nil && u.Presence == domain.PresenceOffDuty {
			status, reason = u.Presence, u.PresenceReason
		}
	}
	return s.set(ctx, userID, status, reason)
}

// Item la mot dong trong PUT /bot/v1/presence.
type Item struct {
	ActorID string
	Status  string // on | off | off_duty
	Reason  string
}

// SetMany cap nhat nhieu nguoi cung luc (CMS goi sang). Tra so dong da cap nhat.
func (s *Service) SetMany(ctx context.Context, items []Item) (int, error) {
	updated := 0
	for _, it := range items {
		actorID := strings.TrimSpace(it.ActorID)
		if actorID == "" {
			continue
		}
		u, err := s.store.Users().GetByActorID(ctx, actorID)
		if err != nil {
			s.log.Warn("presence: khong tim thay actor", "actor_id", actorID)
			continue
		}
		if err := s.set(ctx, u.ID, statusFrom(it.Status, it.Reason), it.Reason); err != nil {
			s.log.Error("presence: cap nhat that bai", "actor_id", actorID, "err", err)
			continue
		}
		updated++
	}
	return updated, nil
}

// statusFrom: "on" -> online; "off" kem ly do -> off_duty (hien chu OFF canh ten);
// "off" khong ly do -> offline (chi la dong tab).
func statusFrom(status, reason string) domain.Presence {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "on", "online":
		return domain.PresenceOnline
	case "off_duty", "offduty", "leave":
		return domain.PresenceOffDuty
	default:
		if strings.TrimSpace(reason) != "" {
			return domain.PresenceOffDuty
		}
		return domain.PresenceOffline
	}
}

func (s *Service) set(ctx context.Context, userID uuid.UUID, status domain.Presence, reason string) error {
	if err := s.store.Users().UpdatePresenceWithReason(ctx, userID, status, reason, time.Now().UTC()); err != nil {
		return err
	}
	s.publish(ctx, userID, status, reason)
	return nil
}

func (s *Service) publish(ctx context.Context, userID uuid.UUID, status domain.Presence, reason string) {
	if s.hub == nil {
		return
	}
	peers, err := s.store.Conversations().ListPeerUserIDs(ctx, userID)
	if err != nil {
		s.log.Error("presence: lay danh sach nguoi lien quan that bai", "err", err)
		return
	}
	if len(peers) == 0 {
		return
	}

	actorID := ""
	if u, err := s.store.Users().GetByID(ctx, userID); err == nil {
		actorID = u.ActorID
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventPresence, "", peers, map[string]any{
		"user_id":  userID,
		"actor_id": actorID,
		"presence": string(status),
		"reason":   reason,
		"online":   status == domain.PresenceOnline,
	}))
}

// SetHeartbeat doi chu ky lam tuoi (chi goi luc wiring). <=0 => tat heartbeat.
func (s *Service) SetHeartbeat(d time.Duration) { s.heartbeat = d }

// Run lam tuoi last_seen_at cho nhung nguoi dang mo WebSocket tren instance nay.
//
// Khong co job nay thi khi process bi kill, cot presence mac ket o "online"
// vinh vien (hook dong ket noi khong chay). Co job nay thi last_seen_at ngung
// moi va User.LivePresence() tu tra offline sau domain.PresenceTTL().
func (s *Service) Run(ctx context.Context) error {
	if s.hub == nil || s.heartbeat <= 0 {
		s.log.Info("presence heartbeat: tat")
		<-ctx.Done()
		return nil
	}
	s.log.Info("presence heartbeat", "chu_ky", s.heartbeat, "het_han_sau", domain.PresenceTTL())

	t := time.NewTicker(s.heartbeat)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			ids := s.hub.OnlineUserIDs()
			if len(ids) == 0 {
				continue
			}
			n, err := s.store.Users().TouchLastSeen(ctx, ids, time.Now().UTC())
			if err != nil {
				s.log.Error("presence heartbeat that bai", "err", err)
				continue
			}
			s.log.Debug("presence heartbeat", "ket_noi", len(ids), "dong_cap_nhat", n)
		}
	}
}
