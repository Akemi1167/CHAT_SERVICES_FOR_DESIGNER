// Package auth xac thuc token FE gui xuong (REST header / WS query / gRPC metadata)
// va dong bo profile user vao DB local.
package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/jwtauth"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// Identity la "ai dang goi request" — duoc gan vao context va dung xuyen suot service layer.
//
// Khi tich hop CMS: token do CMS phat, Chat chi kiem bang JWKS roi anh xa sang
// user local (CMSUserID/OpenID/ActorID giu nguyen gia tri phia CMS).
type Identity struct {
	UserID     uuid.UUID `json:"user_id"`
	ActorID    string    `json:"actor_id,omitempty"`
	CMSUserID  *int64    `json:"cms_user_id,omitempty"`
	OpenID     string    `json:"open_id,omitempty"`
	ExternalID string    `json:"external_id"`
	Name       string    `json:"name"`
	Email      string    `json:"email,omitempty"`
	AvatarURL  string    `json:"avatar_url,omitempty"`
	Role       string    `json:"role,omitempty"`
	TenantID   string    `json:"tenant_id,omitempty"`
}

type ctxKey struct{}

func WithIdentity(ctx context.Context, id *Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

func FromContext(ctx context.Context) (*Identity, error) {
	id, ok := ctx.Value(ctxKey{}).(*Identity)
	if !ok || id == nil {
		return nil, apperr.ErrUnauthenticated
	}
	return id, nil
}

// MustUserID tien cho handler da di qua middleware auth.
func MustUserID(ctx context.Context) (uuid.UUID, error) {
	id, err := FromContext(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return id.UserID, nil
}

// ProfileSource lay ho so day du tu CMS (ten, avatar, to, vai tro).
// cms.Client cai dat interface nay; de nil thi Chat chi dung thong tin trong token.
type ProfileSource interface {
	UsersByCMSID(ctx context.Context, cmsUserIDs []int64) ([]domain.User, error)
}

type Service struct {
	verifier jwtauth.Verifier
	users    repository.UserRepository
	profiles ProfileSource
	log      *slog.Logger
}

func New(verifier jwtauth.Verifier, users repository.UserRepository) *Service {
	return &Service{
		verifier: verifier,
		users:    users,
		log:      slog.Default().With("component", "service.auth"),
	}
}

// SetProfileSource bat dong bo ho so tu CMS (goi trong app wiring khi CMS_ENABLED).
func (s *Service) SetProfileSource(p ProfileSource) { s.profiles = p }

// Authenticate: verify token -> anh xa sang user local -> tra Identity.
// Token co the la "Bearer xxx" hoac token tran.
func (s *Service) Authenticate(ctx context.Context, rawToken string) (*Identity, error) {
	token := jwtauth.ExtractBearer(strings.TrimSpace(rawToken))
	claims, err := s.verifier.Parse(token)
	if err != nil {
		if errors.Is(err, jwtauth.ErrInvalidToken) {
			return nil, apperr.ErrUnauthenticated
		}
		return nil, apperr.ErrUnauthenticated.Wrap(err)
	}

	u, err := s.resolveUser(ctx, claims)
	if err != nil {
		return nil, err
	}

	return &Identity{
		UserID:     u.ID,
		ActorID:    u.ActorID,
		CMSUserID:  u.CMSUserID,
		OpenID:     u.OpenID,
		ExternalID: u.ExternalID,
		Name:       u.DisplayName,
		Email:      u.Email,
		AvatarURL:  u.AvatarURL,
		Role:       u.DisplayRole(),
		TenantID:   claims.TenantID,
	}, nil
}

// resolveUser: token CMS (co cmsUserId) thi dong bo theo dinh danh CMS,
// token dev (HS256) thi giu duong cu theo external_id.
func (s *Service) resolveUser(ctx context.Context, claims *jwtauth.Claims) (*domain.User, error) {
	name := claims.Name
	if name == "" {
		name = claims.Subject
	}

	if claims.CMSUserID == nil {
		return s.users.UpsertByExternalID(ctx, &domain.User{
			ExternalID:  claims.Subject,
			DisplayName: name,
			AvatarURL:   claims.Avatar,
			Email:       claims.Email,
			Role:        claims.Role,
			Presence:    domain.PresenceOffline,
		})
	}

	cmsID := *claims.CMSUserID
	externalID := claims.OpenID
	if externalID == "" {
		externalID = fmt.Sprintf("cms:%d", cmsID)
	}

	u, err := s.users.UpsertFromCMS(ctx, &domain.User{
		CMSUserID:   &cmsID,
		OpenID:      claims.OpenID,
		ActorID:     ActorID(cmsID),
		ExternalID:  externalID,
		DisplayName: name,
		AvatarURL:   claims.Avatar,
		Email:       claims.Email,
		Presence:    domain.PresenceOffline,
	})
	if err != nil {
		return nil, err
	}

	// Lan dau gap user (hoac ho so chua bao gio dong bo) -> lay ho so day du tu CMS,
	// de FE va card visibleTo co san ten/avatar/vai tro ma khong phai goi CMS moi lan.
	if s.profiles != nil && u.CMSProfile == nil {
		if synced, err := s.syncProfile(ctx, cmsID); err != nil {
			s.log.Warn("khong lay duoc ho so tu CMS", "cms_user_id", cmsID, "err", err)
		} else if synced != nil {
			u = synced
		}
	}
	return u, nil
}

func (s *Service) syncProfile(ctx context.Context, cmsUserID int64) (*domain.User, error) {
	profiles, err := s.profiles.UsersByCMSID(ctx, []int64{cmsUserID})
	if err != nil || len(profiles) == 0 {
		return nil, err
	}
	p := profiles[0]
	p.ActorID = ActorID(cmsUserID)
	return s.users.UpsertFromCMS(ctx, &p)
}

// ActorID la dinh danh Chat tra cho CMS (dung trong card.visibleTo, notifications).
// Giu dung quy uoc cua mock CMS: u_<cmsUserId>.
func ActorID(cmsUserID int64) string { return fmt.Sprintf("u_%d", cmsUserID) }

// SetPresence cap nhat trang thai online/offline khi WS connect/disconnect.
func (s *Service) SetPresence(ctx context.Context, userID uuid.UUID, online bool) error {
	p := domain.PresenceOffline
	if online {
		p = domain.PresenceOnline
	}
	return s.users.UpdatePresence(ctx, userID, p, time.Now().UTC())
}
