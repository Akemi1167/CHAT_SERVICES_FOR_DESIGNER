package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type UserRepo struct{ base }

var _ repository.UserRepository = (*UserRepo)(nil)

// UpsertByExternalID: user tu JWT claim -> dong bo profile local.
func (r *UserRepo) UpsertByExternalID(ctx context.Context, u *domain.User) (*domain.User, error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	err := r.db(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "external_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"display_name", "avatar_url", "email", "role", "updated_at",
			}),
		}).
		Create(u).Error
	if err != nil {
		return nil, wrap(err, "user.upsert")
	}
	// Lay lai ban ghi that (truong hop conflict thi ID la cua ban ghi cu).
	var out domain.User
	if err := r.db(ctx).Where("external_id = ?", u.ExternalID).First(&out).Error; err != nil {
		return nil, wrap(err, "user.upsert.reload")
	}
	return &out, nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	var u domain.User
	if err := r.db(ctx).First(&u, "id = ?", id).Error; err != nil {
		return nil, mapErr(err, apperr.NotFound("user_not_found", "Khong tim thay nguoi dung"))
	}
	return &u, nil
}

func (r *UserRepo) ListByIDs(ctx context.Context, ids []uuid.UUID) ([]domain.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []domain.User
	if err := r.db(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, wrap(err, "user.listByIDs")
	}
	return users, nil
}

func (r *UserRepo) UpdatePresence(ctx context.Context, id uuid.UUID, p domain.Presence, seenAt time.Time) error {
	err := r.db(ctx).Model(&domain.User{}).
		Where("id = ?", id).
		Updates(map[string]any{"presence": p, "last_seen_at": seenAt}).Error
	return wrap(err, "user.updatePresence")
}

// ---------------- dinh danh phia CMS ----------------

// UpsertFromCMS tim user theo cms_user_id -> open_id -> external_id.
// Co thi cap nhat ho so (ten, avatar, to, trang thai), khong co thi tao moi.
// CMS la nguon su that nen cac truong o day luon ghi de gia tri cu.
func (r *UserRepo) UpsertFromCMS(ctx context.Context, in *domain.User) (*domain.User, error) {
	existing, err := r.findByCMSIdentity(ctx, in)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if existing == nil {
		if in.ID == uuid.Nil {
			in.ID = uuid.New()
		}
		if in.Status == "" {
			in.Status = "active"
		}
		in.SyncedAt = &now
		if err := r.db(ctx).Create(in).Error; err != nil {
			return nil, wrap(err, "user.upsertFromCMS.create")
		}
		return in, nil
	}

	fields := map[string]any{"synced_at": now}
	setIfNotEmpty := func(key, value string) {
		if value != "" {
			fields[key] = value
		}
	}
	setIfNotEmpty("display_name", in.DisplayName)
	setIfNotEmpty("avatar_url", in.AvatarURL)
	setIfNotEmpty("email", in.Email)
	setIfNotEmpty("username", in.Username)
	setIfNotEmpty("role", in.Role)
	setIfNotEmpty("status", in.Status)
	setIfNotEmpty("open_id", in.OpenID)
	setIfNotEmpty("actor_id", in.ActorID)
	setIfNotEmpty("external_id", in.ExternalID)
	if in.CMSUserID != nil {
		fields["cms_user_id"] = *in.CMSUserID
	}
	if in.CMSProfile != nil {
		fields["cms_profile"] = in.CMSProfile
		fields["is_designer"] = in.IsDesigner
	}

	if err := r.db(ctx).Model(&domain.User{}).Where("id = ?", existing.ID).Updates(fields).Error; err != nil {
		return nil, wrap(err, "user.upsertFromCMS.update")
	}
	return r.GetByID(ctx, existing.ID)
}

func (r *UserRepo) findByCMSIdentity(ctx context.Context, in *domain.User) (*domain.User, error) {
	type lookup struct {
		cond string
		arg  any
		ok   bool
	}
	lookups := []lookup{
		{"cms_user_id = ?", valueOf(in.CMSUserID), in.CMSUserID != nil},
		{"open_id = ?", in.OpenID, in.OpenID != ""},
		{"actor_id = ?", in.ActorID, in.ActorID != ""},
		{"external_id = ?", in.ExternalID, in.ExternalID != ""},
	}
	for _, l := range lookups {
		if !l.ok {
			continue
		}
		var u domain.User
		err := r.db(ctx).First(&u, l.cond, l.arg).Error
		if err == nil {
			return &u, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, wrap(err, "user.findByCMSIdentity")
		}
	}
	return nil, nil
}

func valueOf(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func (r *UserRepo) GetByCMSUserID(ctx context.Context, cmsUserID int64) (*domain.User, error) {
	var u domain.User
	if err := r.db(ctx).First(&u, "cms_user_id = ?", cmsUserID).Error; err != nil {
		return nil, mapErr(err, apperr.NotFound("user_not_found", "Khong tim thay nguoi dung"))
	}
	return &u, nil
}

func (r *UserRepo) GetByOpenID(ctx context.Context, openID string) (*domain.User, error) {
	var u domain.User
	if err := r.db(ctx).First(&u, "open_id = ?", openID).Error; err != nil {
		return nil, mapErr(err, apperr.NotFound("user_not_found", "Khong tim thay nguoi dung"))
	}
	return &u, nil
}

func (r *UserRepo) GetByActorID(ctx context.Context, actorID string) (*domain.User, error) {
	var u domain.User
	if err := r.db(ctx).First(&u, "actor_id = ?", actorID).Error; err != nil {
		return nil, mapErr(err, apperr.NotFound("user_not_found", "Khong tim thay nguoi dung"))
	}
	return &u, nil
}

func (r *UserRepo) ListByOpenIDs(ctx context.Context, openIDs []string) ([]domain.User, error) {
	if len(openIDs) == 0 {
		return nil, nil
	}
	var users []domain.User
	if err := r.db(ctx).Where("open_id IN ?", openIDs).Find(&users).Error; err != nil {
		return nil, wrap(err, "user.listByOpenIDs")
	}
	return users, nil
}

// ListByCMSUserIDs tra ve user theo nhieu cmsUserId trong MOT cau query.
// Dung cho cmssync: dong bo nhom co hang chuc thanh vien, hoi tung nguoi la N+1.
func (r *UserRepo) ListByCMSUserIDs(ctx context.Context, ids []int64) ([]domain.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var users []domain.User
	if err := r.db(ctx).Where("cms_user_id IN ?", ids).Find(&users).Error; err != nil {
		return nil, wrap(err, "user.listByCMSUserIDs")
	}
	return users, nil
}

func (r *UserRepo) ListByActorIDs(ctx context.Context, actorIDs []string) ([]domain.User, error) {
	if len(actorIDs) == 0 {
		return nil, nil
	}
	var users []domain.User
	if err := r.db(ctx).Where("actor_id IN ?", actorIDs).Find(&users).Error; err != nil {
		return nil, wrap(err, "user.listByActorIDs")
	}
	return users, nil
}

// UpdatePresenceWithReason: CMS bao OFF kem ly do (lich OFF, het ca...).
func (r *UserRepo) UpdatePresenceWithReason(ctx context.Context, id uuid.UUID, p domain.Presence, reason string, seenAt time.Time) error {
	err := r.db(ctx).Model(&domain.User{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"presence":        p,
			"presence_reason": reason,
			"last_seen_at":    seenAt,
			"updated_at":      time.Now().UTC(),
		}).Error
	return wrap(err, "user.updatePresenceWithReason")
}

// TouchLastSeen chi cham vao nhung dong DANG la online — khong "hoi sinh"
// nguoi da offline hay dang off_duty.
func (r *UserRepo) TouchLastSeen(ctx context.Context, ids []uuid.UUID, seenAt time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	res := r.db(ctx).Model(&domain.User{}).
		Where("id IN ? AND presence = ?", ids, domain.PresenceOnline).
		Update("last_seen_at", seenAt)
	if res.Error != nil {
		return 0, wrap(res.Error, "user.touchLastSeen")
	}
	return res.RowsAffected, nil
}
