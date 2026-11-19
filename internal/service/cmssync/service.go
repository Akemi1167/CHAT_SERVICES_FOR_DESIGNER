// Package cmssync dong bo nhom Web + ho so user tu CMS sang Chat.
//
// CMS la nguon su that: Chat KHONG tu tao nhom, khong tu them/bot thanh vien.
// Hai duong vao:
//   - poll dinh ky GET /workspace/groups (phong khi bo lo push)
//   - CMS day PUT /bot/v1/groups/:chatId ngay khi co thay doi
package cmssync

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/realtime"
	"github.com/abc-workspace/chat-service/internal/repository"
	"github.com/abc-workspace/chat-service/internal/service/auth"
)

// systemExternalID la user dai dien CMS, dung lam created_by cho nhom do CMS tao.
const systemExternalID = "system:cms"

type Service struct {
	client   *cms.Client
	store    repository.Store
	hub      *realtime.Hub
	interval time.Duration
	log      *slog.Logger

	// Nho id cua user he thong "system:cms": no khong bao gio doi trong mot lan
	// chay, trong khi ApplyGroup can no cho TUNG nhom. Co mutex vi ApplyGroup
	// chay ca o vong dong bo lan o Bot API (PUT /bot/v1/groups/:id).
	systemMu sync.Mutex
	systemID uuid.UUID
}

func New(client *cms.Client, store repository.Store, hub *realtime.Hub, interval time.Duration) *Service {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Service{
		client: client, store: store, hub: hub, interval: interval,
		log: slog.Default().With("component", "service.cmssync"),
	}
}

// Enabled cho biet co bat tich hop CMS khong (de transport tra loi ro rang
// thay vi goi CMS roi bao loi 500).
func (s *Service) Enabled() bool { return s.client != nil && s.client.Enabled() }

// Run dong bo luc khoi dong roi lap lai theo chu ky.
func (s *Service) Run(ctx context.Context) error {
	if n, err := s.SyncAll(ctx); err != nil {
		s.log.Error("dong bo nhom CMS that bai (se thu lai)", "err", err)
	} else {
		s.log.Info("dong bo nhom CMS", "groups", n)
	}

	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			if _, err := s.SyncAll(ctx); err != nil {
				s.log.Error("dong bo nhom CMS that bai", "err", err)
			}
		}
	}
}

// SyncAll keo toan bo nhom Web tu CMS.
func (s *Service) SyncAll(ctx context.Context) (int, error) {
	groups, err := s.client.Groups(ctx)
	if err != nil {
		return 0, err
	}
	for _, g := range groups {
		// Dung applyGroup (chu thuong): vong dong bo khong doc ket qua nen
		// khong can nap lai nhom kem Members/Members.User.
		if _, err := s.applyGroup(ctx, g); err != nil {
			s.log.Error("ap dung nhom that bai", "chat_id", g.ChatID, "err", err)
		}
	}
	return len(groups), nil
}

// ApplyGroup tao/cap nhat nhom va dat lai danh sach thanh vien dung bang CMS,
// roi tra ve nhom da nap day du (kem Members + User) cho caller dung ngay.
func (s *Service) ApplyGroup(ctx context.Context, g cms.Group) (*domain.Conversation, error) {
	conv, err := s.applyGroup(ctx, g)
	if err != nil {
		return nil, err
	}
	return s.store.Conversations().GetByID(ctx, conv.ID)
}

// applyGroup lam toan bo viec ghi; tra ve nhom MOC (chua preload Members).
func (s *Service) applyGroup(ctx context.Context, g cms.Group) (*domain.Conversation, error) {
	if strings.TrimSpace(g.ChatID) == "" {
		return nil, fmt.Errorf("cmssync: thieu chatId")
	}

	systemID, err := s.systemUserID(ctx)
	if err != nil {
		return nil, err
	}

	chatID := g.ChatID
	conv, err := s.store.Conversations().UpsertFromCMS(ctx, &domain.Conversation{
		ExternalID: &chatID,
		Type:       domain.ConversationGroup,
		Name:       g.Name,
		CMSInfo:    g.Info(),
		CreatedBy:  systemID,
	})
	if err != nil {
		return nil, err
	}

	before, err := s.store.Conversations().ListMemberUserIDs(ctx, conv.ID)
	if err != nil {
		return nil, err
	}

	members, err := s.resolveMembers(ctx, g.Members)
	if err != nil {
		return nil, err
	}
	if err := s.store.Conversations().ReplaceMembers(ctx, conv.ID, members); err != nil {
		return nil, err
	}

	after := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		after = append(after, m.UserID)
	}
	// Ai vua vao hoac vua bi go deu can tai lai danh sach nhom.
	s.publishReload(ctx, conv, union(before, after))

	return conv, nil
}

// resolveMembers doi danh sach thanh vien CMS sang thanh vien Chat (tao user neu chua co).
func (s *Service) resolveMembers(ctx context.Context, in []cms.Member) ([]domain.ConversationMember, error) {
	people := make([]cms.Person, 0, len(in))
	for _, m := range in {
		people = append(people, m.Person)
	}
	users, err := s.ResolveUsers(ctx, people)
	if err != nil {
		return nil, err
	}

	byOpen := make(map[string]domain.User, len(users))
	byCMS := make(map[int64]domain.User, len(users))
	for _, u := range users {
		if u.OpenID != "" {
			byOpen[u.OpenID] = u
		}
		if u.CMSUserID != nil {
			byCMS[*u.CMSUserID] = u
		}
	}

	out := make([]domain.ConversationMember, 0, len(in))
	for _, m := range in {
		u, ok := byOpen[m.OpenID]
		if !ok {
			if u, ok = byCMS[m.CMSUserID]; !ok {
				continue
			}
		}
		out = append(out, domain.ConversationMember{
			UserID:    u.ID,
			Role:      roleFromSources(m.Sources),
			Sources:   domain.JSONStrings(m.Sources),
			TeamCodes: domain.JSONStrings(m.TeamCodes),
		})
	}
	return out, nil
}

// roleFromSources: tro ly cua nhom duoc coi la admin trong Chat (chi de kiem quyen noi bo);
// quyen nghiep vu that van do CMS quyet dinh.
func roleFromSources(sources []string) domain.MemberRole {
	for _, s := range sources {
		if s == "assistant" {
			return domain.MemberAdmin
		}
	}
	return domain.MemberNormal
}

// ResolveUsers doi danh sach nguoi cua CMS sang user cua Chat:
// co roi thi dung, chua co thi lay ho so tu CMS roi tao (dung cho Bot API users/resolve).
func (s *Service) ResolveUsers(ctx context.Context, people []cms.Person) ([]domain.User, error) {
	out := make([]domain.User, 0, len(people))
	var missingOpenIDs []string
	missingIdx := map[string]cms.Person{}

	// Tra ca danh sach bang 2 cau IN (...) thay vi 1-2 cau moi nguoi.
	known := s.lookupAll(ctx, people)

	for _, p := range people {
		if u := known.get(p); u != nil {
			out = append(out, *u)
			continue
		}
		if p.OpenID != "" {
			missingOpenIDs = append(missingOpenIDs, p.OpenID)
			missingIdx[p.OpenID] = p
			continue
		}
		// Khong co openId -> tao tu thong tin toi thieu.
		if u, err := s.createFromPerson(ctx, p, nil); err == nil {
			out = append(out, *u)
		}
	}

	if len(missingOpenIDs) == 0 {
		return out, nil
	}

	// Lay ho so day du tu CMS cho nhung nguoi chua co trong Chat.
	profiles, err := s.client.UsersByOpenID(ctx, missingOpenIDs)
	if err != nil {
		s.log.Warn("khong lay duoc ho so tu CMS, tao user toi thieu", "err", err)
	}
	byOpen := make(map[string]domain.User, len(profiles))
	for _, p := range profiles {
		byOpen[p.OpenID] = p
	}

	for openID, person := range missingIdx {
		var profile *domain.User
		if p, ok := byOpen[openID]; ok {
			profile = &p
		}
		u, err := s.createFromPerson(ctx, person, profile)
		if err != nil {
			s.log.Error("tao user that bai", "open_id", openID, "err", err)
			continue
		}
		out = append(out, *u)
	}
	return out, nil
}

// userIndex: ket qua tra hang loat, tra cuu lai trong bo nho.
type userIndex struct {
	byCMSID map[int64]domain.User
	byOpen  map[string]domain.User
}

// get giu dung thu tu uu tien cu: cmsUserId truoc, roi moi den openId.
func (ix userIndex) get(p cms.Person) *domain.User {
	if p.CMSUserID > 0 {
		if u, ok := ix.byCMSID[p.CMSUserID]; ok {
			return &u
		}
	}
	if p.OpenID != "" {
		if u, ok := ix.byOpen[p.OpenID]; ok {
			return &u
		}
	}
	return nil
}

// lookupAll doi N nguoi lay TOI DA 2 cau query (truoc day la toi 2N cau).
// Loi DB khong lam hong dong bo: index rong => coi nhu chua co, di tiep nhanh
// tao moi (UpsertFromCMS van chong trung bang unique index).
func (s *Service) lookupAll(ctx context.Context, people []cms.Person) userIndex {
	ix := userIndex{byCMSID: map[int64]domain.User{}, byOpen: map[string]domain.User{}}

	var (
		cmsIDs  []int64
		openIDs []string
		seenID  = map[int64]bool{}
		seenOpn = map[string]bool{}
	)
	for _, p := range people {
		if p.CMSUserID > 0 && !seenID[p.CMSUserID] {
			seenID[p.CMSUserID] = true
			cmsIDs = append(cmsIDs, p.CMSUserID)
		}
		if p.OpenID != "" && !seenOpn[p.OpenID] {
			seenOpn[p.OpenID] = true
			openIDs = append(openIDs, p.OpenID)
		}
	}

	if users, err := s.store.Users().ListByCMSUserIDs(ctx, cmsIDs); err != nil {
		s.log.Warn("tra user theo cmsUserId that bai", "err", err)
	} else {
		for _, u := range users {
			if u.CMSUserID != nil {
				ix.byCMSID[*u.CMSUserID] = u
			}
		}
	}
	if users, err := s.store.Users().ListByOpenIDs(ctx, openIDs); err != nil {
		s.log.Warn("tra user theo openId that bai", "err", err)
	} else {
		for _, u := range users {
			ix.byOpen[u.OpenID] = u
		}
	}
	return ix
}

func (s *Service) createFromPerson(ctx context.Context, p cms.Person, profile *domain.User) (*domain.User, error) {
	u := domain.User{
		OpenID:      p.OpenID,
		DisplayName: p.Name,
		AvatarURL:   cms.AbsoluteAvatar("", p.AvatarURL),
		Presence:    domain.PresenceOffline,
	}
	if profile != nil {
		u = *profile
	}
	if p.CMSUserID > 0 {
		id := p.CMSUserID
		u.CMSUserID = &id
	}
	if u.DisplayName == "" {
		u.DisplayName = p.Name
	}
	if u.OpenID == "" {
		u.OpenID = p.OpenID
	}
	u.ActorID = ActorIDFor(u.CMSUserID, u.OpenID)
	if u.ExternalID == "" {
		u.ExternalID = u.OpenID
		if u.ExternalID == "" {
			u.ExternalID = u.ActorID
		}
	}
	return s.store.Users().UpsertFromCMS(ctx, &u)
}

// ActorIDFor: co cmsUserId thi dung u_<id> (dung quy uoc CMS mong doi);
// chi co openId thi dung u_<8 ky tu bam> de van on dinh.
func ActorIDFor(cmsUserID *int64, openID string) string {
	if cmsUserID != nil && *cmsUserID > 0 {
		return auth.ActorID(*cmsUserID)
	}
	if openID == "" {
		return ""
	}
	sum := sha1.Sum([]byte(openID))
	return "u_" + hex.EncodeToString(sum[:])[:8]
}

func (s *Service) systemUserID(ctx context.Context) (uuid.UUID, error) {
	s.systemMu.Lock()
	defer s.systemMu.Unlock()
	if s.systemID != uuid.Nil {
		return s.systemID, nil
	}

	u, err := s.store.Users().UpsertByExternalID(ctx, &domain.User{
		ExternalID:  systemExternalID,
		DisplayName: "CMS",
		Role:        "system",
		Presence:    domain.PresenceOffline,
	})
	if err != nil {
		return uuid.Nil, err
	}
	s.systemID = u.ID
	return s.systemID, nil
}

func (s *Service) publishReload(ctx context.Context, conv *domain.Conversation, recipients []uuid.UUID) {
	if s.hub == nil || len(recipients) == 0 {
		return
	}
	external := ""
	if conv.ExternalID != nil {
		external = *conv.ExternalID
	}
	s.hub.Publish(ctx, realtime.NewEvent(realtime.EventReload, conv.ID.String(), recipients, map[string]any{
		"reason":          "group.members_changed",
		"conversation_id": conv.ID,
		"chat_id":         external,
	}))
}

func union(a, b []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(a)+len(b))
	out := make([]uuid.UUID, 0, len(a)+len(b))
	for _, list := range [][]uuid.UUID{a, b} {
		for _, id := range list {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
