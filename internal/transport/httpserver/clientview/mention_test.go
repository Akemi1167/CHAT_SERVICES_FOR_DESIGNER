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
