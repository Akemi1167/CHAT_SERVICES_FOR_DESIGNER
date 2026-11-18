package grpcserver

import (
	chatv1 "github.com/abc-workspace/chat-service/gen/go/chat/v1"
	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/service/task"
)

// Mapping domain <-> proto cho task. Trang thai dung string nen mapping rat mong.

func teamTo(t *domain.Team) *chatv1.Team {
	if t == nil {
		return nil
	}
	return &chatv1.Team{
		Id: t.ID.String(), Code: t.Code, Name: t.Name,
		Kind: string(t.Kind), Enabled: t.Enabled,
	}
}

func teamsTo(in []domain.Team) []*chatv1.Team {
	out := make([]*chatv1.Team, 0, len(in))
	for i := range in {
		out = append(out, teamTo(&in[i]))
	}
	return out
}

func teamMemberTo(m domain.TeamMember) *chatv1.TeamMember {
	return &chatv1.TeamMember{
		Id: m.ID.String(), TeamId: m.TeamID.String(), UserId: m.UserID.String(),
		Role: string(m.Role), Active: m.Active, User: userTo(m.User),
	}
}

func teamMembersTo(in []domain.TeamMember) []*chatv1.TeamMember {
	out := make([]*chatv1.TeamMember, 0, len(in))
	for _, m := range in {
		out = append(out, teamMemberTo(m))
	}
	return out
}

func suggestionsTo(in []task.Suggestion) []*chatv1.AssigneeSuggestion {
	out := make([]*chatv1.AssigneeSuggestion, 0, len(in))
	for _, s := range in {
		out = append(out, &chatv1.AssigneeSuggestion{
			UserId: s.UserID.String(), DisplayName: s.DisplayName, AvatarUrl: s.AvatarURL,
			TeamRole: string(s.Role), Presence: string(s.Presence), OpenTasks: s.OpenTasks,
		})
	}
	return out
}

func assignmentTo(a domain.TaskAssignment) *chatv1.TaskAssignment {
	return &chatv1.TaskAssignment{
		Id:          a.ID.String(),
		TaskId:      a.TaskID.String(),
		UserId:      a.UserID.String(),
		Role:        string(a.Role),
		Status:      string(a.Status),
		Quota:       a.Quota,
		Quantity:    int32(a.Quantity),
		Note:        a.Note,
		AssignedBy:  idPtr(a.AssignedBy),
		AssignedAt:  ts(a.AssignedAt),
		AcceptedAt:  tsPtr(a.AcceptedAt),
		PausedAt:    tsPtr(a.PausedAt),
		HeldAt:      tsPtr(a.HeldAt),
		CompletedAt: tsPtr(a.CompletedAt),
		RevokedAt:   tsPtr(a.RevokedAt),
		User:        userTo(a.User),
	}
}

func revisionTo(r *domain.TaskRevision) *chatv1.TaskRevision {
	if r == nil {
		return nil
	}
	return &chatv1.TaskRevision{
		Id:           r.ID.String(),
		Code:         r.Code,
		TaskId:       r.TaskID.String(),
		AssignmentId: idPtr(r.AssignmentID),
		Reason:       r.Reason,
		RevisionType: r.RevisionType,
		Quantity:     int32(r.Quantity),
		Status:       string(r.Status),
		CreatedBy:    r.CreatedBy.String(),
		CompletedBy:  idPtr(r.CompletedBy),
		CompletedAt:  tsPtr(r.CompletedAt),
		CreatedAt:    ts(r.CreatedAt),
	}
}

func revisionsTo(in []domain.TaskRevision) []*chatv1.TaskRevision {
	out := make([]*chatv1.TaskRevision, 0, len(in))
	for i := range in {
		out = append(out, revisionTo(&in[i]))
	}
	return out
}

func taskEventTo(e domain.TaskEvent) *chatv1.TaskEvent {
	return &chatv1.TaskEvent{
		Id:           e.ID.String(),
		TaskId:       e.TaskID.String(),
		AssignmentId: idPtr(e.AssignmentID),
		RevisionId:   idPtr(e.RevisionID),
		Type:         string(e.Type),
		ActorId:      idPtr(e.ActorID),
		FromStatus:   e.FromStatus,
		ToStatus:     e.ToStatus,
		Message:      e.Message,
		PayloadJson:  e.Payload.String(),
		CreatedAt:    ts(e.CreatedAt),
	}
}

func taskTo(t *domain.Task) *chatv1.Task {
	if t == nil {
		return nil
	}
	assignments := make([]*chatv1.TaskAssignment, 0, len(t.Assignments))
	for _, a := range t.Assignments {
		assignments = append(assignments, assignmentTo(a))
	}
	return &chatv1.Task{
		Id:             t.ID.String(),
		Code:           t.Code,
		ConversationId: t.ConversationID.String(),
		ThreadId:       idPtr(t.ThreadID),
		ParentTaskId:   idPtr(t.ParentTaskID),
		TeamId:         idPtr(t.TeamID),
		Title:          t.Title,
		Description:    t.Description,
		Quantity:       int32(t.Quantity),
		Quota:          t.Quota,
		Deadline:       tsPtr(t.Deadline),
		Priority:       string(t.Priority),
		Status:         string(t.Status),
		CreatedBy:      t.CreatedBy.String(),
		CompletedAt:    tsPtr(t.CompletedAt),
		CreatedAt:      ts(t.CreatedAt),
		MetadataJson:   t.Metadata.String(),
		Assignments:    assignments,
		Revisions:      revisionsTo(t.Revisions),
		Team:           teamTo(t.Team),
	}
}

func tasksTo(in []domain.Task) []*chatv1.Task {
	out := make([]*chatv1.Task, 0, len(in))
	for i := range in {
		out = append(out, taskTo(&in[i]))
	}
	return out
}
