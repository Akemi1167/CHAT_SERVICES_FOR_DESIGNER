package card

import (
	"encoding/json"
	"testing"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestLinkBlocks(t *testing.T) {
	var c domain.JSONMap
	if err := json.Unmarshal([]byte(driveCard), &c); err != nil {
		t.Fatal(err)
	}
	got := LinkBlocks(c)
	if len(got) != 1 || got[0]["title"] != "Google Drive" || len(got[0]["items"].([]any)) != 3 {
		t.Fatalf("muon 1 khoi Google Drive du 3 link, got %v", got)
	}

	// Card v1 chua co Drive / card nil => mang rong (khong phai nil, FE khoi check null).
	for _, card := range []domain.JSONMap{parse(t), nil} {
		if got := LinkBlocks(card); got == nil || len(got) != 0 {
			t.Fatalf("muon [] rong, got %#v", got)
		}
	}

	// Khoi links co visibleTo bi loc truoc khi trich => nguoi ngoai khong thay.
	c["elements"].([]any)[1].(map[string]any)["visibleTo"] = []any{"u_68"}
	if got := LinkBlocks(FilterForViewer(c, "u_99")); len(got) != 0 {
		t.Fatalf("u_99 khong duoc thay khoi links rieng cua u_68, got %v", got)
	}
}

func TestSplitThread(t *testing.T) {
	items := []domain.Message{
		{Content: "Da giao", SenderType: domain.SenderBot},
		{Content: "anh oi", SenderType: domain.SenderUser},
		{Content: "SKY · Nhan task", SenderType: domain.SenderSystem},
		{Content: "ok", SenderType: domain.SenderUser},
	}
	act, disc := splitThread(items)
	if len(act) != 2 || act[0].Content != "Da giao" || act[1].Content != "SKY · Nhan task" {
		t.Fatalf("activity sai: %+v", act)
	}
	if len(disc) != 2 || disc[0].Content != "anh oi" || disc[1].Content != "ok" {
		t.Fatalf("discussion sai: %+v", disc)
	}
}
