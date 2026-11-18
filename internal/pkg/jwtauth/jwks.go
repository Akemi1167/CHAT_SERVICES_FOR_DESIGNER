package jwtauth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWKSVerifier kiem access token do CMS phat: RS256, khoa cong khai lay tu
// GET <CMS>/api/client/v1/workspace/.well-known/jwks.json
//
// Khoa duoc cache; gap kid la thi tai lai ngay (CMS xoay khoa khong can bao truoc).
type JWKSVerifier struct {
	url      string
	issuer   string
	audience string
	tokenTyp string
	leeway   time.Duration
	ttl      time.Duration
	client   *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

type JWKSOption func(*JWKSVerifier)

func WithJWKSHTTPClient(c *http.Client) JWKSOption {
	return func(v *JWKSVerifier) { v.client = c }
}

func WithJWKSLeeway(d time.Duration) JWKSOption {
	return func(v *JWKSVerifier) { v.leeway = d }
}

// NewJWKSVerifier: issuer/audience theo tai lieu CMS la designer-cms / abc-workspace,
// tokenTyp = "access" (claim "typ").
func NewJWKSVerifier(url, issuer, audience string, ttl time.Duration, opts ...JWKSOption) *JWKSVerifier {
	v := &JWKSVerifier{
		url: url, issuer: issuer, audience: audience,
		tokenTyp: "access",
		leeway:   30 * time.Second,
		ttl:      ttl,
		client:   &http.Client{Timeout: 10 * time.Second},
		keys:     map[string]*rsa.PublicKey{},
	}
	for _, o := range opts {
		o(v)
	}
	if v.ttl <= 0 {
		v.ttl = 6 * time.Hour
	}
	return v
}

func (v *JWKSVerifier) Parse(tokenStr string) (*Claims, error) {
	if tokenStr == "" {
		return nil, ErrInvalidToken
	}

	claims := &Claims{}
	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithLeeway(v.leeway),
	}
	if v.issuer != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(v.issuer))
	}
	if v.audience != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(v.audience))
	}

	tok, err := jwt.ParseWithClaims(tokenStr, claims, v.keyFunc, parserOpts...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !tok.Valid {
		return nil, ErrInvalidToken
	}
	// Chi chap nhan access token (CMS con phat refresh token).
	if v.tokenTyp != "" && claims.Typ != v.tokenTyp {
		return nil, fmt.Errorf("%w: typ=%q (can %q)", ErrInvalidToken, claims.Typ, v.tokenTyp)
	}
	if claims.Subject == "" && claims.CMSUserID == nil {
		return nil, fmt.Errorf("%w: thieu sub/cmsUserId", ErrInvalidToken)
	}
	return claims, nil
}

func (v *JWKSVerifier) keyFunc(t *jwt.Token) (any, error) {
	kid, _ := t.Header["kid"].(string)
	if kid == "" {
		return nil, fmt.Errorf("token thieu kid")
	}

	if key := v.cached(kid); key != nil {
		return key, nil
	}
	// kid la hoac cache het han -> tai lai JWKS.
	if err := v.refresh(context.Background()); err != nil {
		return nil, err
	}
	if key := v.cached(kid); key != nil {
		return key, nil
	}
	return nil, fmt.Errorf("khong tim thay khoa kid=%s trong JWKS", kid)
}

func (v *JWKSVerifier) cached(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if time.Since(v.fetchedAt) > v.ttl {
		return nil
	}
	return v.keys[kid]
}

type jwksDoc struct {
	Keys []struct {
		Kty string `json:"kty"`
		N   string `json:"n"`
		E   string `json:"e"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		Use string `json:"use"`
	} `json:"keys"`
}

// Refresh tai lai JWKS (dung luc khoi dong de phat hien sai cau hinh som).
func (v *JWKSVerifier) Refresh(ctx context.Context) error { return v.refresh(ctx) }

func (v *JWKSVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return fmt.Errorf("jwks: tao request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: goi %s: %w", v.url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: %s tra %d", v.url, resp.StatusCode)
	}

	var doc jwksDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("jwks: doc body: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(doc.Keys))
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		pub, err := rsaPublicKey(k.N, k.E)
		if err != nil {
			return fmt.Errorf("jwks: khoa %s: %w", k.Kid, err)
		}
		keys[k.Kid] = pub
	}
	if len(keys) == 0 {
		return fmt.Errorf("jwks: %s khong co khoa RSA nao", v.url)
	}

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func rsaPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		return nil, fmt.Errorf("modulus khong phai base64url: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		return nil, fmt.Errorf("exponent khong phai base64url: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	if e == 0 {
		return nil, fmt.Errorf("exponent rong")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

// ChainVerifier thu lan luot cac verifier: dung cho giai doan chuyen tiep
// (token CMS RS256 la chinh, token dev HS256 van dung duoc cho Postman/CLI).
type ChainVerifier struct {
	verifiers []Verifier
}

func NewChainVerifier(verifiers ...Verifier) *ChainVerifier {
	out := make([]Verifier, 0, len(verifiers))
	for _, v := range verifiers {
		if v != nil {
			out = append(out, v)
		}
	}
	return &ChainVerifier{verifiers: out}
}

func (c *ChainVerifier) Parse(token string) (*Claims, error) {
	var lastErr error
	for _, v := range c.verifiers {
		claims, err := v.Parse(token)
		if err == nil {
			return claims, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = ErrInvalidToken
	}
	return nil, lastErr
}
