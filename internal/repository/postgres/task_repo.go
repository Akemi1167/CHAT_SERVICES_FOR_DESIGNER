package postgres

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/paging"
	"github.com/abc-workspace/chat-service/internal/repository"
)

type TaskRepo struct{ base }

var _ repository.TaskRepository = (*TaskRepo)(nil)

// NextTaskCode sinh ma task dang T-000001 tu sequence cua Postgres.
func (r *TaskRepo) NextTaskCode(ctx context.Context) (string, error) {
	return r.nextCode(ctx, "task_code_seq", "T")
}

func (r *TaskRepo) NextRevisionCode(ctx context.Context) (string, error) {
	return r.nextCode(ctx, "revision_code_seq", "R")
}

func (r *TaskRepo) nextCode(ctx context.Context, seq, prefix string) (string, error) {
	var code string
	err := r.db(ctx).
		Raw(`SELECT ? || '-' || lpad(nextval(?)::text, 6, '0')`, prefix, seq).
		Scan(&code).Error
	if err != nil {
		return "", wrap(err, "task.nextCode")
	}
	return code, nil
}

func (r *TaskRepo) Create(ctx context.Context, t *domain.Task) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	for i := range t.Assignments {
		if t.Assignments[i].ID == uuid.Nil {
			t.Assignments[i].ID = uuid.New()
		}
		t.Assignments[i].TaskID = t.ID
	}
	return wrap(r.db(ctx).Create(t).Error, "task.create")
}

func preloadTask(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Team").
		Preload("Creator").
		Preload("Assignments", func(d *gorm.DB) *gorm.DB {
			return d.Order("task_assignments.assigned_at ASC")
		}).
		Preload("Assignments.User").
		Preload("Revisions", func(d *gorm.DB) *gorm.DB {
			return d.Order("task_revisions.created_at ASC")
		}).
		Preload("Revisions.Creator")
}

func (r *TaskRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Task, error) {
	var t domain.Task
	if err := preloadTask(r.db(ctx)).First(&t, "tasks.id = ?", id).Error; err != nil {
		return nil, mapErr(err, apperr.ErrTaskNotFound)
	}
	return &t, nil
}

func (r *TaskRepo) GetByCode(ctx context.Context, code string) (*domain.Task, error) {
	var t domain.Task
	if err := preloadTask(r.db(ctx)).First(&t, "tasks.code = ?", code).Error; err != nil {
		return nil, mapErr(err, apperr.ErrTaskNotFound)
	}
	return &t, nil
}

func (r *TaskRepo) List(ctx context.Context, f repository.TaskFilter, p paging.Page) ([]domain.Task, error) {
	q := r.db(ctx).Model(&domain.Task{})

	// Chi thay task thuoc hoi thoai minh la thanh vien.
	if f.MemberUserID != uuid.Nil {
		q = q.Where(`tasks.conversation_id IN (
			SELECT conversation_id FROM conversation_members
			WHERE user_id = ? AND left_at IS NULL)`, f.MemberUserID)
	}
	if f.ConversationID != nil {
		q = q.Where("tasks.conversation_id = ?", *f.ConversationID)
	}
	if f.ThreadID != nil {
		q = q.Where("tasks.thread_id = ?", *f.ThreadID)
	}
	if f.TeamID != nil {
		q = q.Where("tasks.team_id = ?", *f.TeamID)
	}
	if f.CreatedBy != nil {
		q = q.Where("tasks.created_by = ?", *f.CreatedBy)
	}
	if len(f.Statuses) > 0 {
		q = q.Where("tasks.status IN ?", f.Statuses)
	}
	// Filter theo card: dung subquery de khong bi nhan dong khi join.
	if f.AssigneeID != nil || len(f.AssignmentStatuses) > 0 {
		sub := r.db(ctx).Model(&domain.TaskAssignment{}).
			Select("task_id").
			Where("status <> ?", domain.AssignRevoked)
		if f.AssigneeID != nil {
			sub = sub.Where("user_id = ?", *f.AssigneeID)
		}
		if len(f.AssignmentStatuses) > 0 {
			sub = sub.Where("status IN ?", f.AssignmentStatuses)
		}
		q = q.Where("tasks.id IN (?)", sub)
	}
	if s := strings.TrimSpace(f.Query); s != "" {
		like := "%" + s + "%"
		q = q.Where("(tasks.title ILIKE ? OR tasks.code ILIKE ? OR tasks.description ILIKE ?)", like, like, like)
	}
	if p.Cursor.Valid {
		q = q.Where("(tasks.created_at, tasks.id) < (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}

	var out []domain.Task
	err := preloadTask(q).
		Order("tasks.created_at DESC, tasks.id DESC").
		Limit(p.FetchLimit()).
		Find(&out).Error
	if err != nil {
		return nil, wrap(err, "task.list")
	}
	return out, nil
}

func (r *TaskRepo) UpdateTask(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["updated_at"] = time.Now().UTC()
	err := r.db(ctx).Model(&domain.Task{}).Where("id = ?", id).Updates(fields).Error
	return wrap(err, "task.update")
}

// ---------- assignments ----------

func (r *TaskRepo) CreateAssignments(ctx context.Context, items []domain.TaskAssignment) error {
	if len(items) == 0 {
		return nil
	}
	now := time.Now().UTC()
	for i := range items {
		if items[i].ID == uuid.Nil {
			items[i].ID = uuid.New()
		}
		if items[i].AssignedAt.IsZero() {
			items[i].AssignedAt = now
		}
		if items[i].Role == "" {
			items[i].Role = domain.AssignPrimary
		}
		if items[i].Status == "" {
			items[i].Status = domain.AssignPending
		}
	}
	return wrap(r.db(ctx).Create(&items).Error, "task.createAssignments")
}

func (r *TaskRepo) GetAssignment(ctx context.Context, id uuid.UUID) (*domain.TaskAssignment, error) {
	var a domain.TaskAssignment
	err := r.db(ctx).Preload("User").First(&a, "id = ?", id).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrAssignmentNotFound)
	}
	return &a, nil
}

func (r *TaskRepo) ListAssignments(ctx context.Context, taskID uuid.UUID) ([]domain.TaskAssignment, error) {
	var out []domain.TaskAssignment
	err := r.db(ctx).Preload("User").
		Where("task_id = ?", taskID).
		Order("assigned_at ASC").Find(&out).Error
	if err != nil {
		return nil, wrap(err, "task.listAssignments")
	}
	return out, nil
}

func (r *TaskRepo) UpdateAssignment(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["updated_at"] = time.Now().UTC()
	err := r.db(ctx).Model(&domain.TaskAssignment{}).Where("id = ?", id).Updates(fields).Error
	return wrap(err, "task.updateAssignment")
}

type openCountRow struct {
	UserID uuid.UUID
	Cnt    int64
}

// CountOpenAssignments: so card dang mo (chua xong, chua bi thu hoi) cua tung user.
func (r *TaskRepo) CountOpenAssignments(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	result := make(map[uuid.UUID]int64, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	for _, id := range userIDs {
		result[id] = 0
	}
	var rows []openCountRow
	err := r.db(ctx).Model(&domain.TaskAssignment{}).
		Select("user_id, COUNT(*) AS cnt").
		Where("user_id IN ?", userIDs).
		Where("status IN ?", []domain.AssignmentStatus{
			domain.AssignPending, domain.AssignInProgress,
			domain.AssignPaused, domain.AssignOnHold,
		}).
		Group("user_id").Scan(&rows).Error
	if err != nil {
		return nil, wrap(err, "task.countOpenAssignments")
	}
	for _, row := range rows {
		result[row.UserID] = row.Cnt
	}
	return result, nil
}

// ---------- revisions ----------

func (r *TaskRepo) CreateRevision(ctx context.Context, rev *domain.TaskRevision) error {
	if rev.ID == uuid.Nil {
		rev.ID = uuid.New()
	}
	if rev.Status == "" {
		rev.Status = domain.RevisionOpen
	}
	return wrap(r.db(ctx).Create(rev).Error, "task.createRevision")
}

func (r *TaskRepo) GetRevision(ctx context.Context, id uuid.UUID) (*domain.TaskRevision, error) {
	var rev domain.TaskRevision
	err := r.db(ctx).Preload("Creator").First(&rev, "id = ?", id).Error
	if err != nil {
		return nil, mapErr(err, apperr.ErrRevisionNotFound)
	}
	return &rev, nil
}

func (r *TaskRepo) ListRevisions(ctx context.Context, taskID uuid.UUID) ([]domain.TaskRevision, error) {
	var out []domain.TaskRevision
	err := r.db(ctx).Preload("Creator").
		Where("task_id = ?", taskID).
		Order("created_at ASC").Find(&out).Error
	if err != nil {
		return nil, wrap(err, "task.listRevisions")
	}
	return out, nil
}

func (r *TaskRepo) UpdateRevision(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["updated_at"] = time.Now().UTC()
	err := r.db(ctx).Model(&domain.TaskRevision{}).Where("id = ?", id).Updates(fields).Error
	return wrap(err, "task.updateRevision")
}

// ---------- events ----------

func (r *TaskRepo) CreateEvent(ctx context.Context, e *domain.TaskEvent) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	return wrap(r.db(ctx).Create(e).Error, "task.createEvent")
}

func (r *TaskRepo) ListEvents(ctx context.Context, taskID uuid.UUID, p paging.Page) ([]domain.TaskEvent, error) {
	q := r.db(ctx).Model(&domain.TaskEvent{}).Where("task_id = ?", taskID)
	if p.Cursor.Valid {
		q = q.Where("(created_at, id) < (?, ?)", p.Cursor.CreatedAt, p.Cursor.ID)
	}
	var out []domain.TaskEvent
	err := q.Preload("Actor").
		Order("created_at DESC, id DESC").
		Limit(p.FetchLimit()).Find(&out).Error
	if err != nil {
		return nil, wrap(err, "task.listEvents")
	}
	return out, nil
}
