package cmssync

import (
	"testing"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestMergeGroupInfo(t *testing.T) {
	existing := domain.JSONMap{"type": "dm", "botId": float64(1), "teams": []any{map[string]any{"id": float64(27)}}}

	t.Run("PUT chi co name + members -> giu type / botId / teams", func(t *testing.T) {
		next := cms.Group{Name: "x"}.Info()
		got := mergeGroupInfo(next, existing, map[string]bool{"name": true, "members": true})
		if got["type"] != "dm" || got["botId"] != float64(1) || len(got["teams"].([]any)) != 1 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("PUT co type -> ghi de", func(t *testing.T) {
		next := cms.Group{Type: "group"}.Info()
		got := mergeGroupInfo(next, existing, map[string]bool{"type": true})
		if got["type"] != "group" || got["botId"] != float64(1) {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("nhom moi -> lay het tu body", func(t *testing.T) {
		next := cms.Group{Type: "dm", BotID: 2}.Info()
		got := mergeGroupInfo(next, nil, map[string]bool{})
		if got["type"] != "dm" || got["botId"] != int64(2) {
			t.Fatalf("got %v", got)
		}
	})
}
