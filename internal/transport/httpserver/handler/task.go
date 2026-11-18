package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
	"github.com/abc-workspace/chat-service/internal/pkg/httpresp"
	"github.com/abc-workspace/chat-service/internal/service/task"
	"github.com/abc-workspace/chat-service/internal/transport/httpserver/middleware"
)

type TaskHandler struct {
	tasks *task.Service
}

func NewTaskHandler(tasks *task.Service) *TaskHandler {
	return &TaskHandler{tasks: tasks}
}

// ---------------- tao / doc ----------------

type createTaskReq struct {
	ConversationID string         `json:"conversation_id"`
	ThreadID       string         `json:"thread_id"`
	Title          string         `json:"title"`
	Description    string         `json:"description"`
	TeamID         string         `json:"team_id"`
	TeamCode       string         `json:"team_code"`
	Quantity       int            `json:"quantity"`
	Quota          string         `json:"quota"`
	Deadline       string         `json:"deadline"`
	Priority       string         `json:"priority"`
	AssigneeIDs    []string       `json:"assignee_ids"`
	ParentTaskID   string         `json:"parent_task_id"`
	Metadata       domain.JSONMap `json:"metadata"`
}

func (h *TaskHandler) Create(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req createTaskReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	convID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_uuid", "conversation_id phai la UUID"))
	}
	threadID, err := optionalUUID(req.ThreadID, "thread_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	teamID, err := optionalUUID(req.TeamID, "team_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	parentID, err := optionalUUID(req.ParentTaskID, "parent_task_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	assignees, err := parseUUIDList(req.AssigneeIDs, "assignee_ids")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	deadline, err := optionalDate(req.Deadline, "deadline")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	t, err := h.tasks.Create(c.UserContext(), task.CreateInput{
		ActorID:        id.UserID,
		ConversationID: convID,
		ThreadID:       threadID,
		Title:          req.Title,
		Description:    req.Description,
		TeamID:         teamID,
		TeamCode:       req.TeamCode,
		Quantity:       req.Quantity,
		Quota:          req.Quota,
		Deadline:       deadline,
		Priority:       domain.TaskPriority(req.Priority),
		AssigneeIDs:    assignees,
		ParentTaskID:   parentID,
		Metadata:       req.Metadata,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.Created(c, t)
}

func (h *TaskHandler) List(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	convID, err := optionalUUID(c.Query("conversation_id"), "conversation_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	threadID, err := optionalUUID(c.Query("thread_id"), "thread_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	teamID, err := optionalUUID(c.Query("team_id"), "team_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	assigneeID, err := optionalUUID(c.Query("assignee_id"), "assignee_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	createdBy, err := optionalUUID(c.Query("created_by"), "created_by")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	var statuses []domain.TaskStatus
	if v := c.Query("status"); v != "" {
		for _, s := range strings.Split(v, ",") {
			if s = strings.TrimSpace(s); s != "" {
				statuses = append(statuses, domain.TaskStatus(s))
			}
		}
	}

	items, info, err := h.tasks.List(c.UserContext(), task.ListInput{
		ActorID:        id.UserID,
		ConversationID: convID,
		ThreadID:       threadID,
		TeamID:         teamID,
		AssigneeID:     assigneeID,
		CreatedBy:      createdBy,
		Statuses:       statuses,
		WaitingAccept:  c.QueryBool("waiting_accept", false),
		MineOnly:       c.QueryBool("mine", false),
		Query:          c.Query("q"),
	}, pageFromQuery(c))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, items, info)
}

func (h *TaskHandler) Get(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	t, err := h.tasks.Get(c.UserContext(), id.UserID, taskID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

func (h *TaskHandler) GetByCode(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	t, err := h.tasks.GetByCode(c.UserContext(), id.UserID, strings.ToUpper(c.Params("code")))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

func (h *TaskHandler) Events(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	items, info, err := h.tasks.Events(c.UserContext(), id.UserID, taskID, pageFromQuery(c))
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OKMeta(c, items, info)
}

// ---------------- vong doi card ----------------

type taskActionReq struct {
	AssignmentID string `json:"assignment_id"`
	Note         string `json:"note"`
	Quota        string `json:"quota"`
	Quantity     int    `json:"quantity"`
}

// action goi 1 trong cac ham vong doi (nhan/tam dung/tiep tuc/treo/hoan thanh).
func (h *TaskHandler) action(c *fiber.Ctx, fn func(ctx *fiber.Ctx, in task.ActionInput) (*domain.Task, error)) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req taskActionReq
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
		}
	}
	assignmentID, err := optionalUUID(req.AssignmentID, "assignment_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	t, err := fn(c, task.ActionInput{
		ActorID:      id.UserID,
		TaskID:       taskID,
		AssignmentID: assignmentID,
		Note:         req.Note,
		Quota:        req.Quota,
		Quantity:     req.Quantity,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

func (h *TaskHandler) Accept(c *fiber.Ctx) error {
	return h.action(c, func(ctx *fiber.Ctx, in task.ActionInput) (*domain.Task, error) {
		return h.tasks.Accept(ctx.UserContext(), in)
	})
}

func (h *TaskHandler) Pause(c *fiber.Ctx) error {
	return h.action(c, func(ctx *fiber.Ctx, in task.ActionInput) (*domain.Task, error) {
		return h.tasks.Pause(ctx.UserContext(), in)
	})
}

func (h *TaskHandler) Resume(c *fiber.Ctx) error {
	return h.action(c, func(ctx *fiber.Ctx, in task.ActionInput) (*domain.Task, error) {
		return h.tasks.Resume(ctx.UserContext(), in)
	})
}

func (h *TaskHandler) Hold(c *fiber.Ctx) error {
	return h.action(c, func(ctx *fiber.Ctx, in task.ActionInput) (*domain.Task, error) {
		return h.tasks.Hold(ctx.UserContext(), in)
	})
}

func (h *TaskHandler) Complete(c *fiber.Ctx) error {
	return h.action(c, func(ctx *fiber.Ctx, in task.ActionInput) (*domain.Task, error) {
		return h.tasks.Complete(ctx.UserContext(), in)
	})
}

// ---------------- phan cong ----------------

type assignReq struct {
	UserIDs []string `json:"user_ids"`
	Role    string   `json:"role"`
	Note    string   `json:"note"`
}

func (h *TaskHandler) AddAssignees(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req assignReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	userIDs, err := parseUUIDList(req.UserIDs, "user_ids")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	t, err := h.tasks.AddAssignees(c.UserContext(), task.AssignInput{
		ActorID: id.UserID, TaskID: taskID, UserIDs: userIDs,
		Role: domain.AssignmentRole(req.Role), Note: req.Note,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

type reassignReq struct {
	AssignmentID string `json:"assignment_id"`
	ToUserID     string `json:"to_user_id"`
	Note         string `json:"note"`
}

func (h *TaskHandler) Reassign(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req reassignReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	toUser, err := uuid.Parse(req.ToUserID)
	if err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_uuid", "to_user_id phai la UUID"))
	}
	assignmentID, err := optionalUUID(req.AssignmentID, "assignment_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	t, err := h.tasks.Reassign(c.UserContext(), task.ReassignInput{
		ActorID: id.UserID, TaskID: taskID, AssignmentID: assignmentID,
		ToUserID: toUser, Note: req.Note,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

// ---------------- dieu chinh / huy ----------------

type updateTaskReq struct {
	Title         *string `json:"title"`
	Description   *string `json:"description"`
	TeamID        *string `json:"team_id"`
	TeamCode      string  `json:"team_code"`
	Quantity      *int    `json:"quantity"`
	Quota         *string `json:"quota"`
	Deadline      *string `json:"deadline"`
	Priority      *string `json:"priority"`
	ClearDeadline bool    `json:"clear_deadline"`
}

func (h *TaskHandler) Update(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req updateTaskReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}

	in := task.UpdateInput{
		ActorID: id.UserID, TaskID: taskID,
		Title: req.Title, Desc: req.Description, TeamCode: req.TeamCode,
		Quantity: req.Quantity, Quota: req.Quota, ClearDeadline: req.ClearDeadline,
	}
	if req.TeamID != nil {
		teamID, err := optionalUUID(*req.TeamID, "team_id")
		if err != nil {
			return httpresp.Fail(c, err)
		}
		in.TeamID = teamID
	}
	if req.Deadline != nil {
		d, err := optionalDate(*req.Deadline, "deadline")
		if err != nil {
			return httpresp.Fail(c, err)
		}
		in.Deadline = d
	}
	if req.Priority != nil {
		p := domain.TaskPriority(*req.Priority)
		in.Priority = &p
	}

	t, err := h.tasks.Update(c.UserContext(), in)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

type cancelReq struct {
	Reason string `json:"reason"`
}

func (h *TaskHandler) Cancel(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req cancelReq
	if len(c.Body()) > 0 {
		_ = c.BodyParser(&req)
	}
	t, err := h.tasks.Cancel(c.UserContext(), id.UserID, taskID, req.Reason)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, t)
}

// ---------------- phieu sua ----------------

type createRevisionReq struct {
	AssignmentID string         `json:"assignment_id"`
	Reason       string         `json:"reason"`
	RevisionType string         `json:"revision_type"`
	Quantity     int            `json:"quantity"`
	Metadata     domain.JSONMap `json:"metadata"`
}

func (h *TaskHandler) CreateRevision(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req createRevisionReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	assignmentID, err := optionalUUID(req.AssignmentID, "assignment_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}

	rev, t, err := h.tasks.CreateRevision(c.UserContext(), task.CreateRevisionInput{
		ActorID: id.UserID, TaskID: taskID, AssignmentID: assignmentID,
		Reason: req.Reason, RevisionType: req.RevisionType,
		Quantity: req.Quantity, Metadata: req.Metadata,
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.Created(c, fiber.Map{"revision": rev, "task": t})
}

func (h *TaskHandler) ListRevisions(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	taskID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	items, err := h.tasks.ListRevisions(c.UserContext(), id.UserID, taskID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, items)
}

func (h *TaskHandler) CompleteRevision(c *fiber.Ctx) error {
	id, err := middleware.Identity(c)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	revID, err := paramUUID(c, "revision_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req taskActionReq
	if len(c.Body()) > 0 {
		_ = c.BodyParser(&req)
	}
	rev, t, err := h.tasks.CompleteRevision(c.UserContext(), id.UserID, revID, req.Note)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, fiber.Map{"revision": rev, "task": t})
}

// ---------------- to / goi y ----------------

func (h *TaskHandler) ListTeams(c *fiber.Ctx) error {
	items, err := h.tasks.ListTeams(c.UserContext())
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, items)
}

type createTeamReq struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

func (h *TaskHandler) CreateTeam(c *fiber.Ctx) error {
	var req createTeamReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	t, err := h.tasks.CreateTeam(c.UserContext(), task.CreateTeamInput{
		Code: req.Code, Name: req.Name, Kind: domain.TeamKind(req.Kind),
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.Created(c, t)
}

type addTeamMembersReq struct {
	UserIDs []string `json:"user_ids"`
	Role    string   `json:"role"`
}

func (h *TaskHandler) AddTeamMembers(c *fiber.Ctx) error {
	teamID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	var req addTeamMembersReq
	if err := c.BodyParser(&req); err != nil {
		return httpresp.Fail(c, apperr.Invalid("invalid_body", "Body khong hop le"))
	}
	userIDs, err := parseUUIDList(req.UserIDs, "user_ids")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	members, err := h.tasks.AddTeamMembers(c.UserContext(), task.AddTeamMembersInput{
		TeamID: teamID, UserIDs: userIDs, Role: domain.TeamMemberRole(req.Role),
	})
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, members)
}

func (h *TaskHandler) ListTeamMembers(c *fiber.Ctx) error {
	teamID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	members, err := h.tasks.ListTeamMembers(c.UserContext(), teamID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, members)
}

func (h *TaskHandler) RemoveTeamMember(c *fiber.Ctx) error {
	teamID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	userID, err := paramUUID(c, "user_id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	if err := h.tasks.RemoveTeamMember(c.UserContext(), teamID, userID); err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, fiber.Map{"removed": true})
}

// Suggest tra danh sach goi y nguoi phu trach cua 1 to (it viec nhat, khong OFF).
func (h *TaskHandler) Suggest(c *fiber.Ctx) error {
	teamID, err := paramUUID(c, "id")
	if err != nil {
		return httpresp.Fail(c, err)
	}
	items, err := h.tasks.SuggestAssignees(c.UserContext(), teamID)
	if err != nil {
		return httpresp.Fail(c, err)
	}
	return httpresp.OK(c, items)
}
