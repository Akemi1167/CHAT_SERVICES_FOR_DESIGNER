package postgres

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm/clause"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type TeamRepo struct{ base }

var _ repository.TeamRepository = (*TeamRepo)(nil)

func (r *TeamRepo) Create(ctx context.Context, t *domain.Team) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return wrap(r.db(ctx).Create(t).Error, "team.create")
}

func (r *TeamRepo) List(ctx context.Context) ([]domain.Team, error) {
	var out []domain.Team
	err := r.db(ctx).Where("enabled = true").Order("code").Find(&out).Error
	if err != nil {
		return nil, wrap(err, "team.list")
	}
	return out, nil
}

func (r *TeamRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Team, error) {
	var t domain.Team
	if err := r.db(ctx).First(&t, "id = ?", id).Error; err != nil {
		return nil, mapErr(err, apperr.ErrTeamNotFound)
	}
	return &t, nil
}

func (r *TeamRepo) GetByCode(ctx context.Context, code string) (*domain.Team, error) {
	var t domain.Team
	if err := r.db(ctx).First(&t, "code = ?", code).Error; err != nil {
		return nil, mapErr(err, apperr.ErrTeamNotFound)
	}
	return &t, nil
}

func (r *TeamRepo) AddMembers(ctx context.Context, members []domain.TeamMember) error {
	if len(members) == 0 {
		return nil
	}
	for i := range members {
		if members[i].ID == uuid.Nil {
			members[i].ID = uuid.New()
		}
		if members[i].Role == "" {
			members[i].Role = domain.TeamStaff
		}
	}
	err := r.db(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "team_id"}, {Name: "user_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"active": true, "updated_at": time.Now().UTC(),
		}),
	}).Create(&members).Error
	return wrap(err, "team.addMembers")
}

func (r *TeamRepo) RemoveMember(ctx context.Context, teamID, userID uuid.UUID) error {
	err := r.db(ctx).Model(&domain.TeamMember{}).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Updates(map[string]any{"active": false, "updated_at": time.Now().UTC()}).Error
	return wrap(err, "team.removeMember")
}

func (r *TeamRepo) ListMembers(ctx context.Context, teamID uuid.UUID) ([]domain.TeamMember, error) {
	var out []domain.TeamMember
	err := r.db(ctx).Preload("User").
		Where("team_id = ? AND active = true", teamID).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "team.listMembers")
	}
	return out, nil
}
