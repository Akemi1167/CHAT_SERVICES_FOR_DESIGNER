package task

import (
	"fmt"
	"strings"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// statusLabel de hien thi tieng Viet trong tin nhan he thong.
var statusLabel = map[domain.TaskStatus]string{
	domain.TaskWaitingAccept: "cho nhan",
	domain.TaskInProgress:    "dang lam",
	domain.TaskPaused:        "tam dung",
	domain.TaskOnHold:        "treo",
	domain.TaskCompleted:     "hoan thanh",
	domain.TaskCancelled:     "da huy",
}

var assignStatusLabel = map[domain.AssignmentStatus]string{
	domain.AssignPending:    "cho nhan",
	domain.AssignInProgress: "dang lam",
	domain.AssignPaused:     "tam dung",
	domain.AssignOnHold:     "treo",
	domain.AssignCompleted:  "hoan thanh",
	domain.AssignRevoked:    "da thu hoi",
}

var assignRoleLabel = map[domain.AssignmentRole]string{
	domain.AssignPrimary: "phu trach",
	domain.AssignSupport: "ho tro",
	domain.AssignEdit:    "edit",
}

// RenderCard sinh noi dung text cua card task (FE co the ve lai tu metadata).
func RenderCard(t *domain.Task) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[%s] %s\n", t.Code, t.Title)

	parts := []string{}
	if t.Team != nil {
		parts = append(parts, "To: "+t.Team.Name)
	}
	parts = append(parts, fmt.Sprintf("So luong: %d", t.Quantity))
	if t.Quota != "" {
		parts = append(parts, "Dinh muc: "+t.Quota)
	}
	if t.Deadline != nil {
		parts = append(parts, "Deadline: "+t.Deadline.Format("02/01/2006 15:04"))
	}
	parts = append(parts, "Trang thai: "+statusLabel[t.Status])
	sb.WriteString(strings.Join(parts, " · ") + "\n")

	active := t.ActiveAssignments()
	if len(active) == 0 {
		sb.WriteString("Chua co nguoi phu trach")
	} else {
		names := make([]string, 0, len(active))
		for _, a := range active {
			name := a.UserID.String()[:8]
			if a.User != nil && a.User.DisplayName != "" {
				name = a.User.DisplayName
			}
			names = append(names, fmt.Sprintf("%s (%s - %s)",
				name, assignRoleLabel[a.Role], assignStatusLabel[a.Status]))
		}
		sb.WriteString("Phu trach: " + strings.Join(names, ", "))
	}
	if t.Description != "" {
		sb.WriteString("\n" + t.Description)
	}
	return sb.String()
}

// cardMetadata la du lieu co cau truc kem card, FE dung de render UI.
func cardMetadata(t *domain.Task) domain.JSONMap {
	assignments := make([]map[string]any, 0, len(t.Assignments))
	for _, a := range t.ActiveAssignments() {
		item := map[string]any{
			"assignment_id": a.ID.String(),
			"user_id":       a.UserID.String(),
			"role":          string(a.Role),
			"status":        string(a.Status),
			"quota":         a.Quota,
			"quantity":      a.Quantity,
		}
		if a.User != nil {
			item["display_name"] = a.User.DisplayName
			item["avatar_url"] = a.User.AvatarURL
		}
		assignments = append(assignments, item)
	}
	meta := domain.JSONMap{
		"kind":        "task_card",
		"task_id":     t.ID.String(),
		"task_code":   t.Code,
		"task_status": string(t.Status),
		"title":       t.Title,
		"quantity":    t.Quantity,
		"quota":       t.Quota,
		"priority":    string(t.Priority),
		"assignments": assignments,
	}
	if t.TeamID != nil {
		meta["team_id"] = t.TeamID.String()
	}
	if t.Team != nil {
		meta["team_name"] = t.Team.Name
	}
	if t.Deadline != nil {
		meta["deadline"] = t.Deadline.UTC()
	}
	return meta
}

func displayName(a *domain.TaskAssignment) string {
	if a.User != nil && a.User.DisplayName != "" {
		return a.User.DisplayName
	}
	return a.UserID.String()[:8]
}
