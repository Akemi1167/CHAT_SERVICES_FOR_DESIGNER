package cmssync

import (
	"testing"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestProfileChanged(t *testing.T) {
	base := domain.User{DisplayName: "Tran Designer", Username: "des3",
		CMSProfile: domain.JSONMap{"assistantOf": []any{}}}

	same := base
	if profileChanged(&base, &same) {
		t.Fatal("khong doi gi ma bao doi")
	}

	renamed := base
	renamed.DisplayName = "Tran Thi Designer"
	if !profileChanged(&base, &renamed) {
		t.Fatal("doi ho ten phai bao")
	}

	assistant := base
	assistant.CMSProfile = domain.JSONMap{"assistantOf": []any{"wg_1"}}
	if !profileChanged(&base, &assistant) {
		t.Fatal("thanh tro ly phai bao")
	}
}
