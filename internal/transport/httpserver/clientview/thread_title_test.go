package clientview

import (
	"strings"
	"testing"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestThreadTitle(t *testing.T) {
	long := strings.Repeat("a", 100)
	cases := []struct {
		name string
		msg  *domain.Message
		want string
	}{
		{"card co code + title", &domain.Message{Card: domain.JSONMap{
			"header": map[string]any{"code": "WEB-20261123-0001", "title": "Banner Tet"}}},
			"WEB-20261123-0001 · Banner Tet"},
		{"card chi co code", &domain.Message{Card: domain.JSONMap{
			"header": map[string]any{"code": "WEB-1"}}}, "WEB-1"},
		{"card kieu cu: title ngoai cung", &domain.Message{Card: domain.JSONMap{"title": "FLOW · Don thu"}}, "FLOW · Don thu"},
		{"tin thuong: dong dau", &domain.Message{Content: "Xin chao\nca nhom"}, "Xin chao"},
		{"tin dai: cat 80 ky tu", &domain.Message{Content: long}, strings.Repeat("a", 80) + "…"},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ThreadTitle(tc.msg); got != tc.want {
				t.Fatalf("ThreadTitle = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPreview(t *testing.T) {
	cases := []struct {
		name string
		msg  *domain.Message
		want string
	}{
		{"nguoi dung", &domain.Message{Content: "kkk", Sender: &domain.User{DisplayName: "JIJON"}}, "JIJON: kkk"},
		{"bot CMS", &domain.Message{Content: "Da giao", BotSender: domain.JSONMap{"name": "Designer Bot"}}, "Designer Bot: Da giao"},
		{"card cua bot", &domain.Message{BotSender: domain.JSONMap{"name": "Designer Bot"},
			Card: domain.JSONMap{"header": map[string]any{"code": "WEB-1", "title": "Banner"}}}, "Designer Bot: WEB-1 · Banner"},
		{"tin he thong: khong ten", &domain.Message{Content: "SKY · Nhan task"}, "SKY · Nhan task"},
		{"chi co tep", &domain.Message{Sender: &domain.User{DisplayName: "An"},
			Attachments: []domain.Attachment{{Kind: "image"}}}, "An: [image]"},
		{"nil", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Preview(tc.msg); got != tc.want {
				t.Fatalf("Preview = %q, want %q", got, tc.want)
			}
		})
	}
}
