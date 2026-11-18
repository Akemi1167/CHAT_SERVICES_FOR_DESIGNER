package domain

import (
	"time"

	"github.com/google/uuid"
)

// TeamKind phan loai to de gan task dung noi (design lam hinh, edit lam video...).
type TeamKind string

const (
	TeamDesign TeamKind = "design"
	TeamEdit   TeamKind = "edit"
	TeamOrder  TeamKind = "order"
	TeamOther  TeamKind = "other"
)

type TeamMemberRole string

const (
	TeamLeader    TeamMemberRole = "leader"    // to truong
	TeamAssistant TeamMemberRole = "assistant" // tro ly
	TeamStaff     TeamMemberRole = "member"
)

func (r TeamMemberRole) Valid() bool {
	switch r {
	case TeamLeader, TeamAssistant, TeamStaff:
		return true
	}
	return false
}

// Team = to / bo phan (dung cho "chon to" khi tao yeu cau va pool goi ho tro).
type Team struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Code      string    `gorm:"size:64;uniqueIndex;not null" json:"code"`
	Name      string    `gorm:"size:255;not null" json:"name"`
	Kind      TeamKind  `gorm:"size:32;not null;default:design" json:"kind"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	Metadata  JSONMap   `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Members []TeamMember `gorm:"foreignKey:TeamID" json:"members,omitempty"`
}

func (Team) TableName() string { return "teams" }

type TeamMember struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	TeamID    uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:ux_team_member" json:"team_id"`
	UserID    uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:ux_team_member" json:"user_id"`
	Role      TeamMemberRole `gorm:"size:32;not null;default:member" json:"role"`
	Active    bool           `gorm:"default:true" json:"active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Team *Team `gorm:"foreignKey:TeamID" json:"team,omitempty"`
}

func (TeamMember) TableName() string { return "team_members" }
