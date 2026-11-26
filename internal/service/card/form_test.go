package card

import (
	"testing"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func TestFormSubmitFields(t *testing.T) {
	actor := map[string]any{"id": "u_11"}
	base := SubmitFormInput{
		FormID: "create_task",
		Values: domain.JSONMap{"teamId": "27", "quantity": "2", "urgencyCode": "normal"},
	}

	t.Run("values chuyen nguyen, ke ca useAI", func(t *testing.T) {
		in := base
		in.Values = domain.JSONMap{"teamId": "27", "quantity": "2", "useAI": false}
		f := formSubmitFields(in, "b_1", "wg_1", actor)
		v := f["values"].(map[string]any)
		if v["quantity"] != "2" || v["useAI"] != false {
			t.Fatalf("values bi doi: %v", v)
		}
		if _, ok := f["ai"]; ok {
			t.Fatal("khong co field ai rieng")
		}
	})

	t.Run("khong co thread", func(t *testing.T) {
		f := formSubmitFields(base, "b_1", "wg_1", actor)
		if _, ok := f["context"].(map[string]any)["threadRootId"]; ok {
			t.Fatal("khong co thread thi khong co threadRootId")
		}
	})

	t.Run("thread", func(t *testing.T) {
		in := base
		id := uuid.New()
		in.ThreadID = &id
		f := formSubmitFields(in, "b_1", "wg_1", actor)
		if f["context"].(map[string]any)["threadRootId"] != id.String() {
			t.Fatalf("context = %v", f["context"])
		}
	})
}
