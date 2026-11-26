package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/pkg/jwtauth"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/google/uuid"
)

const secret = "test-secret"

// usersRepo dong vai repository.UserRepository: ID suy ra tu external_id.
type usersRepo struct{}

func (usersRepo) UpsertByExternalID(_ context.Context, u *domain.User) (*domain.User, error) {
	u.ID = uuid.NewSHA1(uuid.Nil, []byte(u.ExternalID))
	return u, nil
}

func (usersRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	return &domain.User{ID: id}, nil
}

func (usersRepo) ListByIDs(_ context.Context, _ []uuid.UUID) ([]domain.User, error) {
	return nil, nil
}

func (usersRepo) UpdatePresence(_ context.Context, _ uuid.UUID, _ domain.Presence, _ time.Time) error {
	return nil
}

// Cac ham dinh danh CMS: test nay chi dung token dev (HS256) nen khong can thuc thi.
func (r usersRepo) UpsertFromCMS(ctx context.Context, u *domain.User) (*domain.User, error) {
	return r.UpsertByExternalID(ctx, u)
}

func (usersRepo) GetByCMSUserID(_ context.Context, _ int64) (*domain.User, error) {
	return nil, errNotImplemented
}

func (usersRepo) GetByOpenID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errNotImplemented
}

func (usersRepo) GetByActorID(_ context.Context, _ string) (*domain.User, error) {
	return nil, errNotImplemented
}

func (usersRepo) TouchLastSeen(_ context.Context, _ []uuid.UUID, _ time.Time) (int64, error) {
	return 0, nil
}

func (usersRepo) ListByCMSUserIDs(_ context.Context, _ []int64) ([]domain.User, error) {
	return nil, errNotImplemented
}

func (usersRepo) ListByOpenIDs(_ context.Context, _ []string) ([]domain.User, error) {
	return nil, nil
}

func (usersRepo) ListByActorIDs(_ context.Context, _ []string) ([]domain.User, error) {
	return nil, nil
}
func (usersRepo) ListCMSUserIDs(_ context.Context) ([]int64, error) { return nil, nil }

func (usersRepo) UpdatePresenceWithReason(_ context.Context, _ uuid.UUID, _ domain.Presence, _ string, _ time.Time) error {
	return nil
}

var errNotImplemented = errors.New("khong dung trong test nay")

func sign(t *testing.T, sub string) string {
	t.Helper()
	tok, err := jwtauth.Sign(secret, sub, sub, "designer", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

// Khi co CA header Authorization va ?token=:
//   - REST  -> dung header
//   - WS    -> dung query (vi browser/tool hay gan san header cua token khac)
func TestAuthTokenPrecedence(t *testing.T) {
	svc := auth.New(jwtauth.NewVerifier(secret), usersRepo{})
	app := fiber.New()
	app.Get("/rest", Auth(svc), whoami)
	app.Get("/ws", AuthWS(svc), whoami)

	headerTok := sign(t, "nguoi-ngoai")
	queryTok := sign(t, "binh")

	cases := []struct{ path, want string }{
		{"/rest", "nguoi-ngoai"},
		{"/ws", "binh"},
	}
	for _, tc := range cases {
		req := httptest.NewRequest("GET", tc.path+"?token="+queryTok, nil)
		req.Header.Set("Authorization", "Bearer "+headerTok)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("%s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		var env struct {
			Data struct {
				ExternalID string `json:"external_id"`
			} `json:"data"`
		}
		_ = json.Unmarshal(body, &env)
		if env.Data.ExternalID != tc.want {
			t.Errorf("%s: dang nhap bang %q, muon %q (body=%s)", tc.path, env.Data.ExternalID, tc.want, body)
		}
	}
}

func whoami(c *fiber.Ctx) error {
	id, err := Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, id)
}
