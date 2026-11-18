package grpcserver

import (
	"context"
	"strings"

	chatv1 "github.com/abc-workspace/chat-service/gen/go/chat/v1"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/service/auth"
	"github.com/abc-workspace/chat-service/internal/service/task"
)

type TaskHandler struct {
	chatv1.UnimplementedTaskServiceServer
	tasks *task.Service
}

func NewTaskHandler(tasks *task.Service) *TaskHandler {
	return &TaskHandler{tasks: tasks}
}

func (h *TaskHandler) CreateTask(ctx context.Context, req *chatv1.CreateTaskRequest) (*chatv1.TaskResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUID(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	threadID, err := parseUUIDPtr(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	teamID, err := parseUUIDPtr(req.GetTeamId(), "team_id")
	if err != nil {
		return nil, err
	}
	parentID, err := parseUUIDPtr(req.GetParentTaskId(), "parent_task_id")
	if err != nil {
		return nil, err
	}
	assignees, err := parseUUIDs(req.GetAssigneeIds(), "assignee_ids")
	if err != nil {
		return nil, err
	}
	deadline, err := parseTimePtr(req.GetDeadline(), "deadline")
	if err != nil {
		return nil, err
	}
	meta, err := domain.ParseJSONMap(req.GetMetadataJson())
	if err != nil {
		return nil, err
	}

	t, err := h.tasks.Create(ctx, task.CreateInput{
		ActorID:        actor,
		ConversationID: convID,
		ThreadID:       threadID,
		Title:          req.GetTitle(),
		Description:    req.GetDescription(),
		TeamID:         teamID,
		TeamCode:       req.GetTeamCode(),
		Quantity:       int(req.GetQuantity()),
		Quota:          req.GetQuota(),
		Deadline:       deadline,
		Priority:       domain.TaskPriority(req.GetPriority()),
		AssigneeIDs:    assignees,
		ParentTaskID:   parentID,
		Metadata:       meta,
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) GetTask(ctx context.Context, req *chatv1.GetTaskRequest) (*chatv1.TaskResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	if code := strings.TrimSpace(req.GetCode()); code != "" {
		t, err := h.tasks.GetByCode(ctx, actor, strings.ToUpper(code))
		if err != nil {
			return nil, err
		}
		return &chatv1.TaskResponse{Task: taskTo(t)}, nil
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Get(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) ListTasks(ctx context.Context, req *chatv1.ListTasksRequest) (*chatv1.ListTasksResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	convID, err := parseUUIDPtr(req.GetConversationId(), "conversation_id")
	if err != nil {
		return nil, err
	}
	threadID, err := parseUUIDPtr(req.GetThreadId(), "thread_id")
	if err != nil {
		return nil, err
	}
	teamID, err := parseUUIDPtr(req.GetTeamId(), "team_id")
	if err != nil {
		return nil, err
	}
	assigneeID, err := parseUUIDPtr(req.GetAssigneeId(), "assignee_id")
	if err != nil {
		return nil, err
	}
	createdBy, err := parseUUIDPtr(req.GetCreatedBy(), "created_by")
	if err != nil {
		return nil, err
	}
	statuses := make([]domain.TaskStatus, 0, len(req.GetStatuses()))
	for _, s := range req.GetStatuses() {
		if s = strings.TrimSpace(s); s != "" {
			statuses = append(statuses, domain.TaskStatus(s))
		}
	}

	items, info, err := h.tasks.List(ctx, task.ListInput{
		ActorID:        actor,
		ConversationID: convID,
		ThreadID:       threadID,
		TeamID:         teamID,
		AssigneeID:     assigneeID,
		CreatedBy:      createdBy,
		Statuses:       statuses,
		WaitingAccept:  req.GetWaitingAccept(),
		MineOnly:       req.GetMine(),
		Query:          req.GetQuery(),
	}, pageFrom(req.GetPage()))
	if err != nil {
		return nil, err
	}
	return &chatv1.ListTasksResponse{Tasks: tasksTo(items), PageInfo: pageInfoTo(info)}, nil
}

// actionInput chuyen TaskActionRequest -> input cua service (dung chung 5 RPC).
func actionInput(ctx context.Context, req *chatv1.TaskActionRequest) (task.ActionInput, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return task.ActionInput{}, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return task.ActionInput{}, err
	}
	assignmentID, err := parseUUIDPtr(req.GetAssignmentId(), "assignment_id")
	if err != nil {
		return task.ActionInput{}, err
	}
	return task.ActionInput{
		ActorID: actor, TaskID: taskID, AssignmentID: assignmentID,
		Note: req.GetNote(), Quota: req.GetQuota(), Quantity: int(req.GetQuantity()),
	}, nil
}

func (h *TaskHandler) AcceptTask(ctx context.Context, req *chatv1.TaskActionRequest) (*chatv1.TaskResponse, error) {
	in, err := actionInput(ctx, req)
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Accept(ctx, in)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) PauseTask(ctx context.Context, req *chatv1.TaskActionRequest) (*chatv1.TaskResponse, error) {
	in, err := actionInput(ctx, req)
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Pause(ctx, in)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) ResumeTask(ctx context.Context, req *chatv1.TaskActionRequest) (*chatv1.TaskResponse, error) {
	in, err := actionInput(ctx, req)
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Resume(ctx, in)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) HoldTask(ctx context.Context, req *chatv1.TaskActionRequest) (*chatv1.TaskResponse, error) {
	in, err := actionInput(ctx, req)
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Hold(ctx, in)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) CompleteTask(ctx context.Context, req *chatv1.TaskActionRequest) (*chatv1.TaskResponse, error) {
	in, err := actionInput(ctx, req)
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Complete(ctx, in)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) AddAssignees(ctx context.Context, req *chatv1.AddAssigneesRequest) (*chatv1.TaskResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	userIDs, err := parseUUIDs(req.GetUserIds(), "user_ids")
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.AddAssignees(ctx, task.AssignInput{
		ActorID: actor, TaskID: taskID, UserIDs: userIDs,
		Role: domain.AssignmentRole(req.GetRole()), Note: req.GetNote(),
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) ReassignTask(ctx context.Context, req *chatv1.ReassignTaskRequest) (*chatv1.TaskResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	toUser, err := parseUUID(req.GetToUserId(), "to_user_id")
	if err != nil {
		return nil, err
	}
	assignmentID, err := parseUUIDPtr(req.GetAssignmentId(), "assignment_id")
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Reassign(ctx, task.ReassignInput{
		ActorID: actor, TaskID: taskID, AssignmentID: assignmentID,
		ToUserID: toUser, Note: req.GetNote(),
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) UpdateTask(ctx context.Context, req *chatv1.UpdateTaskRequest) (*chatv1.TaskResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}

	in := task.UpdateInput{ActorID: actor, TaskID: taskID, TeamCode: req.GetTeamCode(),
		ClearDeadline: req.GetClearDeadline()}
	if v := req.GetTitle(); v != "" {
		in.Title = &v
	}
	if v := req.GetDescription(); v != "" {
		in.Desc = &v
	}
	if v := req.GetTeamId(); v != "" {
		id, err := parseUUID(v, "team_id")
		if err != nil {
			return nil, err
		}
		in.TeamID = &id
	}
	if v := req.GetQuantity(); v > 0 {
		q := int(v)
		in.Quantity = &q
	}
	if v := req.GetQuota(); v != "" {
		in.Quota = &v
	}
	if v := req.GetDeadline(); v != "" {
		d, err := parseTimePtr(v, "deadline")
		if err != nil {
			return nil, err
		}
		in.Deadline = d
	}
	if v := req.GetPriority(); v != "" {
		p := domain.TaskPriority(v)
		in.Priority = &p
	}

	t, err := h.tasks.Update(ctx, in)
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) CancelTask(ctx context.Context, req *chatv1.CancelTaskRequest) (*chatv1.TaskResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	t, err := h.tasks.Cancel(ctx, actor, taskID, req.GetReason())
	if err != nil {
		return nil, err
	}
	return &chatv1.TaskResponse{Task: taskTo(t)}, nil
}

func (h *TaskHandler) CreateRevision(ctx context.Context, req *chatv1.CreateRevisionRequest) (*chatv1.RevisionResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	assignmentID, err := parseUUIDPtr(req.GetAssignmentId(), "assignment_id")
	if err != nil {
		return nil, err
	}
	meta, err := domain.ParseJSONMap(req.GetMetadataJson())
	if err != nil {
		return nil, err
	}
	rev, t, err := h.tasks.CreateRevision(ctx, task.CreateRevisionInput{
		ActorID: actor, TaskID: taskID, AssignmentID: assignmentID,
		Reason: req.GetReason(), RevisionType: req.GetRevisionType(),
		Quantity: int(req.GetQuantity()), Metadata: meta,
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.RevisionResponse{Revision: revisionTo(rev), Task: taskTo(t)}, nil
}

func (h *TaskHandler) CompleteRevision(ctx context.Context, req *chatv1.CompleteRevisionRequest) (*chatv1.RevisionResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	revID, err := parseUUID(req.GetRevisionId(), "revision_id")
	if err != nil {
		return nil, err
	}
	rev, t, err := h.tasks.CompleteRevision(ctx, actor, revID, req.GetNote())
	if err != nil {
		return nil, err
	}
	return &chatv1.RevisionResponse{Revision: revisionTo(rev), Task: taskTo(t)}, nil
}

func (h *TaskHandler) ListRevisions(ctx context.Context, req *chatv1.ListRevisionsRequest) (*chatv1.ListRevisionsResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	items, err := h.tasks.ListRevisions(ctx, actor, taskID)
	if err != nil {
		return nil, err
	}
	return &chatv1.ListRevisionsResponse{Revisions: revisionsTo(items)}, nil
}

func (h *TaskHandler) ListTaskEvents(ctx context.Context, req *chatv1.ListTaskEventsRequest) (*chatv1.ListTaskEventsResponse, error) {
	actor, err := auth.MustUserID(ctx)
	if err != nil {
		return nil, err
	}
	taskID, err := parseUUID(req.GetTaskId(), "task_id")
	if err != nil {
		return nil, err
	}
	items, info, err := h.tasks.Events(ctx, actor, taskID, pageFrom(req.GetPage()))
	if err != nil {
		return nil, err
	}
	out := make([]*chatv1.TaskEvent, 0, len(items))
	for _, e := range items {
		out = append(out, taskEventTo(e))
	}
	return &chatv1.ListTaskEventsResponse{Events: out, PageInfo: pageInfoTo(info)}, nil
}

// ---------------- teams ----------------

func (h *TaskHandler) ListTeams(ctx context.Context, _ *chatv1.ListTeamsRequest) (*chatv1.ListTeamsResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	items, err := h.tasks.ListTeams(ctx)
	if err != nil {
		return nil, err
	}
	return &chatv1.ListTeamsResponse{Teams: teamsTo(items)}, nil
}

func (h *TaskHandler) CreateTeam(ctx context.Context, req *chatv1.CreateTeamRequest) (*chatv1.TeamResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	t, err := h.tasks.CreateTeam(ctx, task.CreateTeamInput{
		Code: req.GetCode(), Name: req.GetName(), Kind: domain.TeamKind(req.GetKind()),
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.TeamResponse{Team: teamTo(t)}, nil
}

func (h *TaskHandler) AddTeamMembers(ctx context.Context, req *chatv1.AddTeamMembersRequest) (*chatv1.ListTeamMembersResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	teamID, err := parseUUID(req.GetTeamId(), "team_id")
	if err != nil {
		return nil, err
	}
	userIDs, err := parseUUIDs(req.GetUserIds(), "user_ids")
	if err != nil {
		return nil, err
	}
	members, err := h.tasks.AddTeamMembers(ctx, task.AddTeamMembersInput{
		TeamID: teamID, UserIDs: userIDs, Role: domain.TeamMemberRole(req.GetRole()),
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.ListTeamMembersResponse{Members: teamMembersTo(members)}, nil
}

func (h *TaskHandler) RemoveTeamMember(ctx context.Context, req *chatv1.RemoveTeamMemberRequest) (*chatv1.RemoveTeamMemberResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	teamID, err := parseUUID(req.GetTeamId(), "team_id")
	if err != nil {
		return nil, err
	}
	userID, err := parseUUID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}
	if err := h.tasks.RemoveTeamMember(ctx, teamID, userID); err != nil {
		return nil, err
	}
	return &chatv1.RemoveTeamMemberResponse{}, nil
}

func (h *TaskHandler) ListTeamMembers(ctx context.Context, req *chatv1.ListTeamMembersRequest) (*chatv1.ListTeamMembersResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	teamID, err := parseUUID(req.GetTeamId(), "team_id")
	if err != nil {
		return nil, err
	}
	members, err := h.tasks.ListTeamMembers(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &chatv1.ListTeamMembersResponse{Members: teamMembersTo(members)}, nil
}

func (h *TaskHandler) SuggestAssignees(ctx context.Context, req *chatv1.SuggestAssigneesRequest) (*chatv1.SuggestAssigneesResponse, error) {
	if _, err := auth.MustUserID(ctx); err != nil {
		return nil, err
	}
	teamID, err := parseUUID(req.GetTeamId(), "team_id")
	if err != nil {
		return nil, err
	}
	items, err := h.tasks.SuggestAssignees(ctx, teamID)
	if err != nil {
		return nil, err
	}
	return &chatv1.SuggestAssigneesResponse{Suggestions: suggestionsTo(items)}, nil
}
