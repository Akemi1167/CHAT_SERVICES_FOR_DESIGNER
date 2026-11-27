package clientview

import (
	"testing"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestMessageMentions(t *testing.T) {
	m := &domain.Message{ID: uuid.New(), Mentions: domain.JSONStrings{"u_55", "u_9"}}

	v := Message(m, "u_55")
	if v["mentionedMe"] != true || v["mentionAll"] != false {
		t.Fatalf("view = %v", v)
	}
	if got := v["mentions"].([]string); len(got) != 2 {
		t.Fatalf("mentions = %v", got)
	}
	if Message(m, "u_1")["mentionedMe"] != false {
		t.Fatal("u_1 khong duoc nhac")
	}

	plain := Message(&domain.Message{ID: uuid.New()}, "u_55")
	if _, ok := plain["mentions"]; ok {
		t.Fatal("tin khong nhac ai thi khong tra mentions")
	}
}

func TestMessageMentionUsers(t *testing.T) {
	des := domain.User{ID: uuid.New(), ActorID: "u_55", DisplayName: "Trần Designer", Username: "des3", IsDesigner: true,
		CMSProfile: domain.JSONMap{"webTeams": []any{map[string]any{"teamId": 27}}}}
	tl := domain.User{ID: uuid.New(), ActorID: "u_9", DisplayName: "Trịnh Văn Long", Role: "admin"}
	m := &domain.Message{ID: uuid.New(), Mentions: domain.JSONStrings{"u_9", "u_55", "u_404"},
		MentionUsers: []domain.User{des, tl, {ActorID: "u_404"}}}

	got := Message(m, "u_55")["mentionUsers"].([]map[string]any)
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
	// dung thu tu mentions, khong theo thu tu MentionUsers
	if got[0]["actorId"] != "u_9" || got[0]["name"] != "Trịnh Văn Long" || got[0]["role"] != "admin" {
		t.Errorf("[0] = %v", got[0])
	}
	if got[1]["actorId"] != "u_55" || got[1]["username"] != "des3" || got[1]["role"] != "designer" {
		t.Errorf("[1] = %v", got[1])
	}
	// user khong con trong Chat: van co actorId, ten rong
	if got[2]["actorId"] != "u_404" || got[2]["name"] != "" {
		t.Errorf("[2] = %v", got[2])
	}
}
