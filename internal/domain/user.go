package domain

import (
	"time"

	"github.com/google/uuid"
)

// User la ban sao (profile cache) cua user tu he thong auth/HRM.
// Service chat khong tu dang ky user: user duoc upsert tu claim cua JWT.
type User struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ExternalID string    `gorm:"size:128;uniqueIndex;not null" json:"external_id"`

	// ---- dinh danh phia CMS (CMS la nguon su that ve tai khoan) ----
	// CMSUserID = users.id ben CMS; OpenID = cau noi CMS <-> Chat;
	// ActorID = dinh danh Chat tra cho CMS (dung trong card.visibleTo, notifications).
	CMSUserID  *int64 `gorm:"uniqueIndex" json:"cms_user_id,omitempty"`
	OpenID     string `gorm:"size:128;uniqueIndex" json:"open_id,omitempty"`
	ActorID    string `gorm:"size:64;uniqueIndex" json:"actor_id,omitempty"`
	Username   string `gorm:"size:128" json:"username,omitempty"`
	Status     string `gorm:"size:32;default:active" json:"status,omitempty"`
	IsDesigner bool   `gorm:"default:false" json:"is_designer"`
	// CMSProfile giu webTeams / assistantOf / botAdminOf de suy vai tro hien thi
	// va de FE lay thong tin nguoi dung ma khong phai goi lai CMS.
	CMSProfile JSONMap    `gorm:"type:jsonb" json:"cms_profile,omitempty"`
	SyncedAt   *time.Time `json:"synced_at,omitempty"`

	DisplayName string     `gorm:"size:255;not null" json:"display_name"`
	AvatarURL   string     `gorm:"size:1024" json:"avatar_url"`
	Email       string     `gorm:"size:255;index" json:"email"`
	Role        string     `gorm:"size:64;index" json:"role"` // admin | order | designer | ...
	Presence    Presence   `gorm:"size:32;default:offline" json:"presence"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	Metadata    JSONMap    `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (User) TableName() string { return "users" }

// DisplayRole suy vai tro hien thi theo quy uoc CMS (chi de hien thi,
// quyen thao tac luon do CMS quyet dinh).
func (u *User) DisplayRole() string {
	if u.Role == "admin" || len(u.cmsList("botAdminOf")) > 0 {
		return "admin"
	}
	if len(u.cmsList("assistantOf")) > 0 {
		return "assistant"
	}
	if u.IsDesigner || len(u.cmsList("webTeams")) > 0 {
		return "designer"
	}
	// Chua co ho so CMS (token dev hoac he thong auth khac) thi giu nguyen role
	// trong token, dung suy thanh "order".
	if u.CMSProfile == nil && u.Role != "" {
		return u.Role
	}
	return "order"
}

func (u *User) cmsList(key string) []any {
	if u.CMSProfile == nil {
		return nil
	}
	v, _ := u.CMSProfile[key].([]any)
	return v
}

// AssistantOf tra danh sach chatId ma user lam tro ly.
func (u *User) AssistantOf() []string {
	out := make([]string, 0)
	for _, v := range u.cmsList("assistantOf") {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
