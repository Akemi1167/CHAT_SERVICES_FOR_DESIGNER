package dblog

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

// Filter: moi field zero = khong loc.
type Filter struct {
	MinLevel  string // debug|info|warn|error — lay level nay TRO LEN
	Component string
	RequestID string
	Path      string // chua chuoi nay
	Q         string // tim trong msg va attrs (khong phan biet hoa thuong)
	StatusMin int    // vd 400 = moi request loi
	Since     *time.Time
	Until     *time.Time
	BeforeID  int64 // phan trang: lay dong co id < BeforeID
	Limit     int   // mac dinh 100, toi da 500
}

// Reader doc app_logs cho API debug.
type Reader struct{ db *gorm.DB }

func NewReader(db *gorm.DB) *Reader {
	return &Reader{db: db.Session(&gorm.Session{Logger: gormlogger.Discard})}
}

// levelsFrom: "warn" -> [WARN ERROR].
func levelsFrom(min string) []string {
	all := []string{"DEBUG", "INFO", "WARN", "ERROR"}
	name := LevelName(min)
	for i, l := range all {
		if l == name {
			return all[i:]
		}
	}
	return nil
}

// List tra log moi nhat truoc. nextBefore > 0 = con trang sau (truyen vao BeforeID).
func (r *Reader) List(ctx context.Context, f Filter) (rows []domain.AppLog, nextBefore int64, err error) {
	if f.Limit <= 0 {
		f.Limit = 100
	}
	if f.Limit > 500 {
		f.Limit = 500
	}
	if f.MinLevel != "" && LevelName(f.MinLevel) == "" {
		return nil, 0, apperr.Invalid("invalid_level", "level phai la debug | info | warn | error")
	}

	q := r.db.WithContext(ctx).Model(&domain.AppLog{})
	if lv := levelsFrom(f.MinLevel); lv != nil {
		q = q.Where("level IN ?", lv)
	}
	if f.Component != "" {
		q = q.Where("component = ?", f.Component)
	}
	if f.RequestID != "" {
		q = q.Where("request_id = ?", f.RequestID)
	}
	if s := strings.TrimSpace(f.Path); s != "" {
		q = q.Where("path ILIKE ?", "%"+s+"%")
	}
	if s := strings.TrimSpace(f.Q); s != "" {
		like := "%" + s + "%"
		q = q.Where("(msg ILIKE ? OR attrs::text ILIKE ?)", like, like)
	}
	if f.StatusMin > 0 {
		q = q.Where("status >= ?", f.StatusMin)
	}
	if f.Since != nil {
		q = q.Where("created_at >= ?", *f.Since)
	}
	if f.Until != nil {
		q = q.Where("created_at <= ?", *f.Until)
	}
	if f.BeforeID > 0 {
		q = q.Where("id < ?", f.BeforeID)
	}

	if err := q.Order("id DESC").Limit(f.Limit + 1).Find(&rows).Error; err != nil {
		return nil, 0, apperr.Internal("db_error", "Khong doc duoc app_logs").Wrap(err)
	}
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		nextBefore = rows[len(rows)-1].ID
	}
	return rows, nextBefore, nil
}
