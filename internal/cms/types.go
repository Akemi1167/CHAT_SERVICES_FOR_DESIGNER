// Package cms noi chuyen voi CMS/Task Engine (Node):
//   - Workspace API (Chat -> CMS): nhom, thanh vien, ho so user  [service key]
//   - Webhook      (Chat -> CMS): form.submit, card.action, message.created, user.updated  [HMAC]
//
// CMS la nguon su that cho tai khoan / nhom / don. Chat KHONG doc database CMS.
package cms

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// Envelope: moi response Workspace API deu boc dang nay.
type Envelope[T any] struct {
	StatusCode int    `json:"statusCode"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Element    T      `json:"element"`
}

type Team struct {
	ID   int64  `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type Person struct {
	CMSUserID int64  `json:"cmsUserId"`
	OpenID    string `json:"openId"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatarUrl"`
}

// Member: thanh vien hieu luc cua nhom Web, CMS tinh san.
// sources: team | assistant | manual | invite (mot nguoi co the co nhieu nguon).
type Member struct {
	Person
	Sources   []string `json:"sources"`
	TeamCodes []string `json:"teamCodes"`
}

// Group = nhom Web (chatId dang wg_<hex>). Chat khong tu tao nhom nay.
type Group struct {
	ChatID         string   `json:"chatId"`
	Name           string   `json:"name"`
	BotID          int64    `json:"botId"`
	Timezone       string   `json:"timezone"`
	IsOrderHub     bool     `json:"isOrderHub"`
	OrderTeamCodes []string `json:"orderTeamCodes"`
	Teams          []Team   `json:"teams"`
	Assistants     []Person `json:"assistants"`
	Members        []Member `json:"members"`
}

// Info gom cac truong phu cua nhom, luu vao conversations.cms_info de FE dung lai.
func (g Group) Info() domain.JSONMap {
	teams := make([]any, 0, len(g.Teams))
	for _, t := range g.Teams {
		teams = append(teams, map[string]any{"id": t.ID, "code": t.Code, "name": t.Name})
	}
	return domain.JSONMap{
		"botId":          g.BotID,
		"timezone":       g.Timezone,
		"isOrderHub":     g.IsOrderHub,
		"orderTeamCodes": g.OrderTeamCodes,
		"teams":          teams,
	}
}

// User: ho so tra ve tu GET /workspace/users.
type User struct {
	CMSUserID   int64    `json:"cmsUserId"`
	OpenID      string   `json:"openId"`
	Name        string   `json:"name"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	AvatarURL   string   `json:"avatarUrl"`
	Status      string   `json:"status"`
	Role        string   `json:"role"`
	IsDesigner  bool     `json:"isDesigner"`
	WebTeams    []any    `json:"webTeams"`
	AssistantOf []string `json:"assistantOf"`
	BotAdminOf  []any    `json:"botAdminOf"`
}

// ToDomain doi ho so CMS sang user cua Chat (chua co ID noi bo).
// baseURL dung de ghep avatarUrl tuong doi (/api/avatars/...).
func (u User) ToDomain(baseURL string) domain.User {
	id := u.CMSUserID
	return domain.User{
		CMSUserID:   &id,
		OpenID:      u.OpenID,
		ExternalID:  externalID(u),
		Username:    u.Username,
		DisplayName: u.Name,
		Email:       u.Email,
		AvatarURL:   AbsoluteAvatar(baseURL, u.AvatarURL),
		Status:      u.Status,
		Role:        u.Role,
		IsDesigner:  u.IsDesigner,
		CMSProfile: domain.JSONMap{
			"webTeams":    u.WebTeams,
			"assistantOf": toAnySlice(u.AssistantOf),
			"botAdminOf":  u.BotAdminOf,
		},
	}
}

func externalID(u User) string {
	if u.OpenID != "" {
		return u.OpenID
	}
	return "cms:" + strconv.FormatInt(u.CMSUserID, 10)
}

// AbsoluteAvatar ghep duong dan tuong doi cua CMS voi base URL.
func AbsoluteAvatar(baseURL, avatar string) string {
	if avatar == "" || strings.HasPrefix(avatar, "http://") || strings.HasPrefix(avatar, "https://") {
		return avatar
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(avatar, "/")
}

// ---- webhook ----

// Toast la thong bao CMS muon UI hien.
type Toast struct {
	Type string `json:"type"` // success | error | info
	Text string `json:"text"`
}

// WebhookResponse: ket qua CMS tra cho event dong bo (form.submit, card.action).
type WebhookResponse struct {
	OK          bool            `json:"ok"`
	Toast       *Toast          `json:"toast,omitempty"`
	CloseForm   bool            `json:"closeForm"`
	Card        json.RawMessage `json:"card,omitempty"`
	CardVersion int             `json:"cardVersion,omitempty"`
	Deduped     bool            `json:"deduped,omitempty"`
}

// CardMap doi card tra ve thanh JSONMap de luu jsonb.
func (r WebhookResponse) CardMap() (domain.JSONMap, error) {
	if len(r.Card) == 0 {
		return nil, nil
	}
	var m domain.JSONMap
	if err := json.Unmarshal(r.Card, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func toAnySlice(in []string) []any {
	out := make([]any, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	return out
}
