package domain

import (
	"time"

	"github.com/google/uuid"
)

// ---------------- enum + state machine ----------------

// TaskStatus la trang thai TONG cua task, duoc SUY RA tu cac assignment
// (xem DeriveTaskStatus). Khong set tay tru khi huy.
type TaskStatus string

const (
	TaskWaitingAccept TaskStatus = "waiting_accept" // cho nhan
	TaskInProgress    TaskStatus = "in_progress"
	TaskPaused        TaskStatus = "paused"    // tam dung
	TaskOnHold        TaskStatus = "on_hold"   // treo
	TaskCompleted     TaskStatus = "completed" // hoan thanh
	TaskCancelled     TaskStatus = "cancelled"
)

// AssignmentStatus la trang thai card cua TUNG nguoi.
type AssignmentStatus string

const (
	AssignPending    AssignmentStatus = "pending" // da giao, chua nhan
	AssignInProgress AssignmentStatus = "in_progress"
	AssignPaused     AssignmentStatus = "paused"
	AssignOnHold     AssignmentStatus = "on_hold"
	AssignCompleted  AssignmentStatus = "completed"
	AssignRevoked    AssignmentStatus = "revoked" // bi thu hoi khi doi nguoi phu trach
)

// AssignmentRole phan biet nguoi phu trach chinh / ho tro / lam video.
type AssignmentRole string

const (
	AssignPrimary AssignmentRole = "primary"
	AssignSupport AssignmentRole = "support"
	AssignEdit    AssignmentRole = "edit"
)

func (r AssignmentRole) Valid() bool {
	switch r {
	case AssignPrimary, AssignSupport, AssignEdit:
		return true
	}
	return false
}

type RevisionStatus string

const (
	RevisionOpen      RevisionStatus = "open"
	RevisionCompleted RevisionStatus = "completed"
	RevisionCancelled RevisionStatus = "cancelled"
)

type TaskPriority string

const (
	PriorityLow    TaskPriority = "low"
	PriorityNormal TaskPriority = "normal"
	PriorityHigh   TaskPriority = "high"
	PriorityUrgent TaskPriority = "urgent"
)

func (p TaskPriority) Valid() bool {
	switch p {
	case PriorityLow, PriorityNormal, PriorityHigh, PriorityUrgent:
		return true
	}
	return false
}

// TaskEventType dung cho bang task_events (lich su task).
type TaskEventType string

const (
	EventTaskCreated       TaskEventType = "task.created"
	EventTaskAssigned      TaskEventType = "task.assigned"
	EventTaskAccepted      TaskEventType = "task.accepted"
	EventTaskPaused        TaskEventType = "task.paused"
	EventTaskResumed       TaskEventType = "task.resumed"
	EventTaskHeld          TaskEventType = "task.held"
	EventTaskCompleted     TaskEventType = "task.completed"
	EventTaskReassigned    TaskEventType = "task.reassigned"
	EventTaskUpdated       TaskEventType = "task.updated"
	EventTaskCancelled     TaskEventType = "task.cancelled"
	EventRevisionCreated   TaskEventType = "revision.created"
	EventRevisionCompleted TaskEventType = "revision.completed"
	EventAssignmentRevoked TaskEventType = "assignment.revoked"
)

// AllowedAssignmentTransitions dinh nghia state machine cua card.
// Them trang thai moi = them 1 dong o day, service khong phai sua logic.
var AllowedAssignmentTransitions = map[AssignmentStatus][]AssignmentStatus{
	AssignPending:    {AssignInProgress, AssignRevoked},
	AssignInProgress: {AssignPaused, AssignOnHold, AssignCompleted, AssignRevoked},
	AssignPaused:     {AssignInProgress, AssignOnHold, AssignRevoked},
	AssignOnHold:     {AssignInProgress, AssignRevoked},
	AssignCompleted:  {AssignInProgress}, // mo lai khi co phieu sua
	AssignRevoked:    {},
}

func (s AssignmentStatus) CanMoveTo(next AssignmentStatus) bool {
	for _, allowed := range AllowedAssignmentTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// DeriveTaskStatus suy trang thai task tu danh sach card.
// Uu tien: in_progress > paused > on_hold > waiting_accept.
func DeriveTaskStatus(assignments []TaskAssignment) TaskStatus {
	var active []TaskAssignment
	for _, a := range assignments {
		if a.Status != AssignRevoked {
			active = append(active, a)
		}
	}
	if len(active) == 0 {
		return TaskWaitingAccept
	}

	var pending, inProgress, paused, onHold, completed int
	for _, a := range active {
		switch a.Status {
		case AssignPending:
			pending++
		case AssignInProgress:
			inProgress++
		case AssignPaused:
			paused++
		case AssignOnHold:
			onHold++
		case AssignCompleted:
			completed++
		}
	}

	switch {
	case completed == len(active):
		return TaskCompleted
	case inProgress > 0:
		return TaskInProgress
	case paused > 0:
		return TaskPaused
	case onHold > 0:
		return TaskOnHold
	case pending == len(active):
		return TaskWaitingAccept
	default:
		return TaskInProgress
	}
}

// ---------------- entity ----------------

type Task struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Code           string     `gorm:"size:32;uniqueIndex;not null" json:"code"`
	ConversationID uuid.UUID  `gorm:"type:uuid;not null;index" json:"conversation_id"`
	ThreadID       *uuid.UUID `gorm:"type:uuid;index" json:"thread_id,omitempty"`
	ParentTaskID   *uuid.UUID `gorm:"type:uuid" json:"parent_task_id,omitempty"`
	SubmissionID   *uuid.UUID `gorm:"type:uuid" json:"submission_id,omitempty"`
	TeamID         *uuid.UUID `gorm:"type:uuid;index" json:"team_id,omitempty"`

	Title       string       `gorm:"size:500;not null" json:"title"`
	Description string       `gorm:"type:text" json:"description"`
	Quantity    int          `gorm:"default:1" json:"quantity"`
	Quota       string       `gorm:"size:64" json:"quota"`
	Deadline    *time.Time   `json:"deadline,omitempty"`
	Priority    TaskPriority `gorm:"size:16;default:normal" json:"priority"`
	Status      TaskStatus   `gorm:"size:32;not null;default:waiting_accept;index" json:"status"`

	CreatedBy   uuid.UUID  `gorm:"type:uuid;not null;index" json:"created_by"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Metadata    JSONMap    `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`

	Assignments []TaskAssignment `gorm:"foreignKey:TaskID" json:"assignments,omitempty"`
	Revisions   []TaskRevision   `gorm:"foreignKey:TaskID" json:"revisions,omitempty"`
	Team        *Team            `gorm:"foreignKey:TeamID" json:"team,omitempty"`
	Creator     *User            `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

func (Task) TableName() string { return "tasks" }

// ActiveAssignments bo cac card da bi thu hoi.
func (t *Task) ActiveAssignments() []TaskAssignment {
	out := make([]TaskAssignment, 0, len(t.Assignments))
	for _, a := range t.Assignments {
		if a.Status != AssignRevoked {
			out = append(out, a)
		}
	}
	return out
}

// AssignmentOf tra card cua 1 user (nil neu khong co).
func (t *Task) AssignmentOf(userID uuid.UUID) *TaskAssignment {
	for i := range t.Assignments {
		if t.Assignments[i].UserID == userID && t.Assignments[i].Status != AssignRevoked {
			return &t.Assignments[i]
		}
	}
	return nil
}

// OpenRevisions dem so phieu sua chua hoan thanh.
func (t *Task) OpenRevisions() int {
	n := 0
	for _, r := range t.Revisions {
		if r.Status == RevisionOpen {
			n++
		}
	}
	return n
}

type TaskAssignment struct {
	ID       uuid.UUID        `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID   uuid.UUID        `gorm:"type:uuid;not null;index" json:"task_id"`
	UserID   uuid.UUID        `gorm:"type:uuid;not null;index" json:"user_id"`
	Role     AssignmentRole   `gorm:"size:32;not null;default:primary" json:"role"`
	Status   AssignmentStatus `gorm:"size:32;not null;default:pending;index" json:"status"`
	Quota    string           `gorm:"size:64" json:"quota"`
	Quantity int              `gorm:"default:0" json:"quantity"`
	Note     string           `gorm:"type:text" json:"note"`

	AssignedBy  *uuid.UUID `gorm:"type:uuid" json:"assigned_by,omitempty"`
	AssignedAt  time.Time  `json:"assigned_at"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	PausedAt    *time.Time `json:"paused_at,omitempty"`
	HeldAt      *time.Time `json:"held_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`

	Metadata  JSONMap   `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Task *Task `gorm:"foreignKey:TaskID" json:"-"`
}

func (TaskAssignment) TableName() string { return "task_assignments" }

// TaskRevision = phieu sua. Moi phieu hoan thanh rieng.
type TaskRevision struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Code         string         `gorm:"size:32;uniqueIndex;not null" json:"code"`
	TaskID       uuid.UUID      `gorm:"type:uuid;not null;index" json:"task_id"`
	AssignmentID *uuid.UUID     `gorm:"type:uuid" json:"assignment_id,omitempty"`
	Reason       string         `gorm:"type:text;not null" json:"reason"`
	RevisionType string         `gorm:"size:64" json:"revision_type"`
	Quantity     int            `gorm:"default:1" json:"quantity"`
	Status       RevisionStatus `gorm:"size:32;not null;default:open;index" json:"status"`
	SubmissionID *uuid.UUID     `gorm:"type:uuid" json:"submission_id,omitempty"`
	MessageID    *uuid.UUID     `gorm:"type:uuid" json:"message_id,omitempty"`
	CreatedBy    uuid.UUID      `gorm:"type:uuid;not null" json:"created_by"`
	CompletedBy  *uuid.UUID     `gorm:"type:uuid" json:"completed_by,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	Metadata     JSONMap        `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`

	Creator *User `gorm:"foreignKey:CreatedBy" json:"creator,omitempty"`
}

func (TaskRevision) TableName() string { return "task_revisions" }

// TaskEvent la lich su thay doi (ai lam gi, luc nao).
type TaskEvent struct {
	ID           uuid.UUID     `gorm:"type:uuid;primaryKey" json:"id"`
	TaskID       uuid.UUID     `gorm:"type:uuid;not null;index" json:"task_id"`
	AssignmentID *uuid.UUID    `gorm:"type:uuid" json:"assignment_id,omitempty"`
	RevisionID   *uuid.UUID    `gorm:"type:uuid" json:"revision_id,omitempty"`
	Type         TaskEventType `gorm:"size:48;not null" json:"type"`
	ActorID      *uuid.UUID    `gorm:"type:uuid" json:"actor_id,omitempty"`
	FromStatus   string        `gorm:"size:32" json:"from_status,omitempty"`
	ToStatus     string        `gorm:"size:32" json:"to_status,omitempty"`
	Message      string        `gorm:"type:text" json:"message,omitempty"`
	Payload      JSONMap       `gorm:"type:jsonb" json:"payload,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`

	Actor *User `gorm:"foreignKey:ActorID" json:"actor,omitempty"`
}

func (TaskEvent) TableName() string { return "task_events" }
