package cms

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/domain"
)

const (
	headerServiceKey = "X-Chat-Service-Key"
	headerTimestamp  = "X-Abc-Timestamp"
	headerSignature  = "X-Abc-Signature"

	EventSchema = "abc.event/v1"

	EventFormSubmit    = "form.submit"
	EventCardAction    = "card.action"
	EventMessageCreate = "message.created"
	EventUserUpdated   = "user.updated"
)

type Client struct {
	cfg  config.CMS
	http *http.Client
	log  *slog.Logger
}

func New(cfg config.CMS) *Client {
	timeout := cfg.HTTPTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
		log:  slog.Default().With("component", "cms.client"),
	}
}

func (c *Client) Enabled() bool { return c.cfg.Enabled }

// ---------------- Workspace API (service key) ----------------

// Groups lay toan bo nhom Web + thanh vien hieu luc.
func (c *Client) Groups(ctx context.Context) ([]Group, error) {
	var env Envelope[struct {
		Groups []Group `json:"groups"`
	}]
	if err := c.get(ctx, "/groups", nil, &env); err != nil {
		return nil, err
	}
	return env.Element.Groups, nil
}

// GroupMembers lay thanh vien cua mot nhom.
func (c *Client) GroupMembers(ctx context.Context, chatID string) ([]Member, error) {
	var env Envelope[struct {
		ChatID  string   `json:"chatId"`
		Members []Member `json:"members"`
	}]
	if err := c.get(ctx, "/groups/"+url.PathEscape(chatID)+"/members", nil, &env); err != nil {
		return nil, err
	}
	return env.Element.Members, nil
}

// Users tra ho so theo cmsUserId va/hoac openId (toi da 200 moi lan).
func (c *Client) Users(ctx context.Context, cmsUserIDs []int64, openIDs []string) ([]User, error) {
	if len(cmsUserIDs) == 0 && len(openIDs) == 0 {
		return nil, nil
	}
	q := url.Values{}
	if len(cmsUserIDs) > 0 {
		ids := make([]string, 0, len(cmsUserIDs))
		for _, id := range cmsUserIDs {
			ids = append(ids, strconv.FormatInt(id, 10))
		}
		q.Set("ids", strings.Join(ids, ","))
	}
	if len(openIDs) > 0 {
		q.Set("openIds", strings.Join(openIDs, ","))
	}

	var env Envelope[struct {
		Users []User `json:"users"`
	}]
	if err := c.get(ctx, "/users", q, &env); err != nil {
		return nil, err
	}
	return env.Element.Users, nil
}

// UsersUpdatedSince tra ho so doi SAU moc since (ISO), xep cu -> moi, toi da limit (<= 200).
// Chat luu NextSince lam moc lan sau; HasMore = con nua, goi tiep ngay.
func (c *Client) UsersUpdatedSince(ctx context.Context, since string, limit int) (*UsersPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	q := url.Values{}
	q.Set("updatedSince", since)
	q.Set("limit", strconv.Itoa(limit))

	var env Envelope[UsersPage]
	if err := c.get(ctx, "/users", q, &env); err != nil {
		return nil, err
	}
	return &env.Element, nil
}

// AbsoluteURL ghep duong dan tuong doi cua CMS (/api/avatars/...) voi base URL.
func (c *Client) AbsoluteURL(path string) string { return AbsoluteAvatar(c.cfg.BaseURL, path) }

// ProfilesToDomain doi ho so CMS sang user Chat (ghep avatar tuong doi voi base URL).
func (c *Client) ProfilesToDomain(users []User) []domain.User { return c.toDomain(users) }

// UsersByCMSID cai dat auth.ProfileSource.
func (c *Client) UsersByCMSID(ctx context.Context, ids []int64) ([]domain.User, error) {
	users, err := c.Users(ctx, ids, nil)
	if err != nil {
		return nil, err
	}
	return c.toDomain(users), nil
}

// UsersByOpenID dung cho Bot API users/resolve (nguoi chua tung dang nhap Chat).
func (c *Client) UsersByOpenID(ctx context.Context, openIDs []string) ([]domain.User, error) {
	users, err := c.Users(ctx, nil, openIDs)
	if err != nil {
		return nil, err
	}
	return c.toDomain(users), nil
}

func (c *Client) toDomain(users []User) []domain.User {
	out := make([]domain.User, 0, len(users))
	for _, u := range users {
		out = append(out, u.ToDomain(c.cfg.BaseURL))
	}
	return out
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	endpoint := c.cfg.WorkspaceBaseURL + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("cms: tao request %s: %w", path, err)
	}
	req.Header.Set(headerServiceKey, c.cfg.ServiceKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cms: goi %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cms: %s tra %d: %.200s", path, resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("cms: doc response %s: %w", path, err)
	}
	return nil
}

// ---------------- Webhook (HMAC) ----------------

// SendEvent ky va gui event sang CMS. Dung cho ca event dong bo
// (form.submit, card.action — UI cho ket qua) va bat dong bo.
//
// Chu ky: hex(HMAC_SHA256(CHAT_SERVICE_KEY, "<timestamp>.<rawBody>")) tren ĐÚNG
// bytes da gui (khong serialize lai).
func (c *Client) SendEvent(ctx context.Context, eventType string, fields map[string]any, timeout time.Duration) (*WebhookResponse, error) {
	payload := BuildEvent(eventType, fields)
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cms: marshal event: %w", err)
	}
	return c.sendRaw(ctx, raw, timeout)
}

// SendRawEvent gui lai payload da luu (outbox) — giu nguyen eventId de CMS chong trung.
func (c *Client) SendRawEvent(ctx context.Context, payload domain.JSONMap, timeout time.Duration) (*WebhookResponse, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cms: marshal event: %w", err)
	}
	return c.sendRaw(ctx, raw, timeout)
}

func (c *Client) sendRaw(ctx context.Context, raw []byte, timeout time.Duration) (*WebhookResponse, error) {
	if timeout <= 0 {
		timeout = c.cfg.WebhookTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("cms: tao request webhook: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(headerTimestamp, ts)
	req.Header.Set(headerSignature, Sign(c.cfg.ServiceKey, ts, raw))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cms: goi webhook: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cms: webhook tra %d: %.200s", resp.StatusCode, body)
	}

	var out WebhookResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("cms: doc response webhook: %w", err)
	}
	return &out, nil
}

// Sign tao chu ky HMAC cho webhook.
func Sign(serviceKey, timestamp string, rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(serviceKey))
	mac.Write([]byte(timestamp + "."))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}

// BuildEvent dung khung chung abc.event/v1.
func BuildEvent(eventType string, fields map[string]any) domain.JSONMap {
	ev := domain.JSONMap{
		"schema":    EventSchema,
		"eventId":   "evt_" + uuid.NewString(),
		"type":      eventType,
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range fields {
		ev[k] = v
	}
	return ev
}

// Actor dung trong event: CMS nhan user theo cmsUserId hoac openId.
func Actor(actorID, openID string, cmsUserID *int64, name string) map[string]any {
	a := map[string]any{"id": actorID, "name": name}
	if openID != "" {
		a["openId"] = openID
	}
	if cmsUserID != nil {
		a["cmsUserId"] = *cmsUserID
	}
	return a
}
