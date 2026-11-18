// Package jwtauth xac thuc token do FE gui kem (header, query ?token=, hoac WS subprotocol).
// Chi verify + doc claim; viec phat token thuoc service auth khac.
package jwtauth

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims toi thieu service chat can. Them field tuy he thong auth cua ban.
//
// Token do CMS phat (xem docs/10-cms-integration.md) co them:
//
//	{"sub":"92","cmsUserId":92,"openId":"local:<uuid>","name":"...",
//	 "typ":"access","iss":"designer-cms","aud":"abc-workspace"}
type Claims struct {
	jwt.RegisteredClaims
	Name     string `json:"name,omitempty"`
	Email    string `json:"email,omitempty"`
	Avatar   string `json:"avatar,omitempty"`
	Role     string `json:"role,omitempty"`
	TenantID string `json:"tenant_id,omitempty"`

	// ---- do CMS phat ----
	CMSUserID *int64 `json:"cmsUserId,omitempty"`
	OpenID    string `json:"openId,omitempty"`
	Typ       string `json:"typ,omitempty"`
}

// Verifier la hop dong kiem token, de doi giua token CMS (RS256 + JWKS)
// va token dev (HS256) ma khong anh huong tang service.
type Verifier interface {
	Parse(token string) (*Claims, error)
}

// HS256Verifier kiem token ky bang secret dung chung (chi dung cho dev/test).
type HS256Verifier struct {
	secret   []byte
	issuer   string
	audience string
	leeway   time.Duration
}

type Option func(*HS256Verifier)

func WithIssuer(iss string) Option   { return func(v *HS256Verifier) { v.issuer = iss } }
func WithAudience(aud string) Option { return func(v *HS256Verifier) { v.audience = aud } }
func WithLeeway(d time.Duration) Option {
	return func(v *HS256Verifier) { v.leeway = d }
}

func NewVerifier(secret string, opts ...Option) *HS256Verifier {
	v := &HS256Verifier{secret: []byte(secret), leeway: 30 * time.Second}
	for _, o := range opts {
		o(v)
	}
	return v
}

var ErrInvalidToken = errors.New("invalid token")

func (v *HS256Verifier) Parse(tokenStr string) (*Claims, error) {
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithLeeway(v.leeway),
	}
	if v.issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(v.issuer))
	}
	if v.audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(v.audience))
	}

	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return v.secret, nil
	}, parserOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !tok.Valid || claims.Subject == "" {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// ExtractBearer lay token tu gia tri header "Authorization".
func ExtractBearer(header string) string {
	if header == "" {
		return ""
	}
	if len(header) > 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return strings.TrimSpace(header)
}

// Sign chi dung cho test/dev (lenh `make token`).
func Sign(secret, subject, name, role string, ttl time.Duration) (string, error) {
	now := time.Now()
	c := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Name: name,
		Role: role,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
}
