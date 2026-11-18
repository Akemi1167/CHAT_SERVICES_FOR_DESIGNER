package task

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

// ---------------- to / bo phan ----------------

func (s *Service) ListTeams(ctx context.Context) ([]domain.Team, error) {
	return s.store.Teams().List(ctx)
}

type CreateTeamInput struct {
	Code string
	Name string
	Kind domain.TeamKind
}

func (s *Service) CreateTeam(ctx context.Context, in CreateTeamInput) (*domain.Team, error) {
	code := strings.ToLower(strings.TrimSpace(in.Code))
	name := strings.TrimSpace(in.Name)
	if code == "" || name == "" {
		return nil, apperr.Invalid("team_code_name_required", "To phai co code va name")
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.TeamOther
	}
	t := &domain.Team{ID: uuid.New(), Code: code, Name: name, Kind: kind, Enabled: true}
	if err := s.store.Teams().Create(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

type AddTeamMembersInput struct {
	TeamID  uuid.UUID
	UserIDs []uuid.UUID
	Role    domain.TeamMemberRole
}

func (s *Service) AddTeamMembers(ctx context.Context, in AddTeamMembersInput) ([]domain.TeamMember, error) {
	if _, err := s.store.Teams().GetByID(ctx, in.TeamID); err != nil {
		return nil, err
	}
	role := in.Role
	if !role.Valid() {
		role = domain.TeamStaff
	}
	members := make([]domain.TeamMember, 0, len(in.UserIDs))
	for _, uid := range uniqueIDs(in.UserIDs) {
		members = append(members, domain.TeamMember{
			ID: uuid.New(), TeamID: in.TeamID, UserID: uid, Role: role, Active: true,
		})
	}
	if len(members) == 0 {
		return nil, apperr.Invalid("team_members_required", "Phai co it nhat 1 user_id")
	}
	if err := s.store.Teams().AddMembers(ctx, members); err != nil {
		return nil, err
	}
	return s.store.Teams().ListMembers(ctx, in.TeamID)
}

func (s *Service) RemoveTeamMember(ctx context.Context, teamID, userID uuid.UUID) error {
	if _, err := s.store.Teams().GetByID(ctx, teamID); err != nil {
		return err
	}
	return s.store.Teams().RemoveMember(ctx, teamID, userID)
}

func (s *Service) ListTeamMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMember, error) {
	if _, err := s.store.Teams().GetByID(ctx, teamID); err != nil {
		return nil, err
	}
	return s.store.Teams().ListMembers(ctx, teamID)
}

func (s *Service) resolveTeam(ctx context.Context, id *uuid.UUID, code string) (*domain.Team, error) {
	if id != nil {
		return s.store.Teams().GetByID(ctx, *id)
	}
	if c := strings.ToLower(strings.TrimSpace(code)); c != "" {
		return s.store.Teams().GetByCode(ctx, c)
	}
	return nil, nil
}

// ---------------- goi y nguoi phu trach ----------------

// Suggestion la 1 dong goi y: thanh vien to + so viec dang mo + trang thai online/OFF.
type Suggestion struct {
	UserID      uuid.UUID             `json:"user_id"`
	DisplayName string                `json:"display_name"`
	AvatarURL   string                `json:"avatar_url,omitempty"`
	Role        domain.TeamMemberRole `json:"team_role"`
	Presence    domain.Presence       `json:"presence"`
	OpenTasks   int64                 `json:"open_tasks"`
}

// SuggestAssignees xep hang thanh vien cua to: uu tien nguoi KHONG OFF va it viec nhat.
// Admin/order van co quyen doi nguoi khac (FE chi hien goi y).
func (s *Service) SuggestAssignees(ctx context.Context, teamID uuid.UUID) ([]Suggestion, error) {
	members, err := s.store.Teams().ListMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return nil, nil
	}

	userIDs := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
	}
	counts, err := s.store.Tasks().CountOpenAssignments(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	out := make([]Suggestion, 0, len(members))
	for _, m := range members {
		sug := Suggestion{
			UserID:    m.UserID,
			Role:      m.Role,
			OpenTasks: counts[m.UserID],
			Presence:  domain.PresenceOffline,
		}
		if m.User != nil {
			sug.DisplayName = m.User.DisplayName
			sug.AvatarURL = m.User.AvatarURL
			sug.Presence = m.User.Presence
		}
		out = append(out, sug)
	}

	// Sap xep: nguoi khong OFF truoc, roi it viec nhat, roi theo ten cho on dinh.
	sort.SliceStable(out, func(i, j int) bool {
		offI := out[i].Presence == domain.PresenceOffDuty
		offJ := out[j].Presence == domain.PresenceOffDuty
		if offI != offJ {
			return !offI
		}
		if out[i].OpenTasks != out[j].OpenTasks {
			return out[i].OpenTasks < out[j].OpenTasks
		}
		return out[i].DisplayName < out[j].DisplayName
	})
	return out, nil
}

// SuggestAssignee tra nguoi phu hop nhat (nil neu to khong co ai kha dung).
func (s *Service) SuggestAssignee(ctx context.Context, teamID uuid.UUID) (*Suggestion, error) {
	list, err := s.SuggestAssignees(ctx, teamID)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	// Bo qua nguoi dang OFF neu con nguoi khac.
	for i := range list {
		if list[i].Presence != domain.PresenceOffDuty {
			return &list[i], nil
		}
	}
	return &list[0], nil
}
