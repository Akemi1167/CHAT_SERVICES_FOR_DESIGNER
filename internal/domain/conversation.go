package domain

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"time"

	"github.com/google/uuid"
)

type Conversation struct {
	ID        uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	Type      ConversationType `gorm:"size:32;not null;index" json:"type"`
	Name      string           `gorm:"size:255" json:"name"`
	AvatarURL string           `gorm:"size:1024" json:"avatar_url"`

	// DirectKey dam bao 2 user chi co dung 1 cuoc chat rieng.
	// NULL voi group (unique index cho phep nhieu NULL tren Postgres).
	DirectKey *string `gorm:"size:64;uniqueIndex" json:"-"`

	// ExternalID = chatId cua CMS (wg_<hex>). Nhom tao trong Chat thi rong.
	// Source: cms = do CMS quan ly (Chat khong tu them/bot thanh vien), local = tao trong Chat.
	ExternalID *string    `gorm:"size:64;uniqueIndex" json:"external_id,omitempty"`
	Source     string     `gorm:"size:16;default:local" json:"source"`
	CMSInfo    JSONMap    `gorm:"type:jsonb" json:"cms_info,omitempty"`
	SyncedAt   *time.Time `json:"synced_at,omitempty"`

	CreatedBy     uuid.UUID  `gorm:"type:uuid;index" json:"created_by"`
	LastMessageID *uuid.UUID `gorm:"type:uuid" json:"last_message_id,omitempty"`
	LastMessageAt *time.Time `gorm:"index" json:"last_message_at,omitempty"`
	Metadata      JSONMap    `gorm:"type:jsonb" json:"metadata,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Members []ConversationMember `gorm:"foreignKey:ConversationID" json:"members,omitempty"`

	// Field tinh toan, khong luu DB.
	UnreadCount int64 `gorm:"-" json:"unread_count"`
	// MentionCount = so tin chua doc nhac toi nguoi dang xem (dau "@" canh so chua doc).
	MentionCount int64 `gorm:"-" json:"mention_count"`
	// AvatarSet: chi dung khi dong bo tu CMS — true = CMS co gui avatarUrl
	// (ke ca rong = go avatar), false = khong gui -> giu avatar_url hien tai.
	AvatarSet bool `gorm:"-" json:"-"`
}

// CMSTypeDM: cms_info.type cua nhom "chat rieng voi bot" (CMS gui type = "dm").
const CMSTypeDM = "dm"

// IsBotDM: chat rieng voi bot — chi bot (CMS qua Bot API) gui tin, nguoi dung chi doc.
func (c *Conversation) IsBotDM() bool {
	if c == nil || c.CMSInfo == nil {
		return false
	}
	t, _ := c.CMSInfo["type"].(string)
	return t == CMSTypeDM
}

// ClientType: loai nhom tra cho UI — "dm" (chat rieng voi bot) hoac "group".
func (c *Conversation) ClientType() string {
	if c.IsBotDM() {
		return CMSTypeDM
	}
	return string(ConversationGroup)
}

// BotID tra botId cua CMS gan voi nhom (cms_info.botId), 0 neu khong co.
func (c *Conversation) BotID() int64 {
	if c == nil || c.CMSInfo == nil {
		return 0
	}
	return toInt64(c.CMSInfo["botId"])
}

func (Conversation) TableName() string { return "conversations" }

// Nguon quan ly nhom.
const (
	SourceLocal = "local" // nhom tao trong Chat
	SourceCMS   = "cms"   // nhom Web do CMS quan ly (Chat khong tu them/bot thanh vien)
)

// IsCMS cho biet nhom do CMS quan ly.
func (c *Conversation) IsCMS() bool { return c.Source == SourceCMS }

type ConversationMember struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ConversationID uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:ux_conv_member" json:"conversation_id"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:ux_conv_member;index" json:"user_id"`
	Role           MemberRole `gorm:"size:32;not null;default:member" json:"role"`

	LastReadMessageID *uuid.UUID `gorm:"type:uuid" json:"last_read_message_id,omitempty"`
	LastReadAt        *time.Time `json:"last_read_at,omitempty"`
	Muted             bool       `gorm:"default:false" json:"muted"`
	// PinnedAt: nguoi nay ghim nhom len dau thanh ben (rieng tung nguoi). nil = khong ghim.
	PinnedAt *time.Time `json:"pinned_at,omitempty"`

	// Sources / TeamCodes do CMS tinh: team | assistant | manual | invite.
	Sources   JSONStrings `gorm:"type:jsonb" json:"sources,omitempty"`
	TeamCodes JSONStrings `gorm:"type:jsonb" json:"team_codes,omitempty"`

	JoinedAt time.Time  `json:"joined_at"`
	LeftAt   *time.Time `json:"left_at,omitempty"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}

func (ConversationMember) TableName() string { return "conversation_members" }

// BuildDirectKey sinh key on dinh cho cap user (khong phu thuoc thu tu).
func BuildDirectKey(a, b uuid.UUID) string {
	ids := []string{a.String(), b.String()}
	sort.Strings(ids)
	sum := sha1.Sum([]byte(ids[0] + "|" + ids[1]))
	return hex.EncodeToString(sum[:])
}
