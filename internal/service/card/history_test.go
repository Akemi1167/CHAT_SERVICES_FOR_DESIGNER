package card

import (
	"testing"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestActionLabel(t *testing.T) {
	c := parse(t)
	c["actions"] = []any{map[string]any{"action": "split_task", "label": "Chia task"}}

	cases := []struct {
		name   string
		card   domain.JSONMap
		action string
		want   string
	}{
		{"lay text cua nut", c, "accept_task", "Nhan task"},
		{"lay label khi khong co text", c, "split_task", "Chia task"},
		{"action khong co trong card => ten action", c, "pause", "pause"},
		{"card nil => ten action", nil, "accept_task", "accept_task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := actionLabel(tc.card, tc.action); got != tc.want {
				t.Fatalf("actionLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFormHistoryText(t *testing.T) {
	cases := []struct {
		name  string
		toast *cms.Toast
		want  string
	}{
		{"co toast => dung cau cua CMS", &cms.Toast{Type: "success", Text: "Đã gửi yêu cầu WEB-20261122-0014"},
			"SKY · Đã gửi yêu cầu WEB-20261122-0014"},
		{"toast rong => cau chung", &cms.Toast{Type: "success"}, "SKY · da gui form create_task"},
		{"khong co toast => cau chung", nil, "SKY · da gui form create_task"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formHistoryText("SKY", "create_task", tc.toast); got != tc.want {
				t.Fatalf("formHistoryText = %q, want %q", got, tc.want)
			}
		})
	}
}
