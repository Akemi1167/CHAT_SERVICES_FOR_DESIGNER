package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/domain"
	"github.com/abc-workspace/chat-service/internal/repository"
)

// parseQuery chay conversationFilterFromQuery voi query string cho truoc.
func parseQuery(t *testing.T, qs string) (repository.ConversationFilter, error) {
	t.Helper()
	app := fiber.New()
	var (
		got repository.ConversationFilter
		err error
	)
	app.Get("/x", func(c *fiber.Ctx) error {
		got, err = conversationFilterFromQuery(c)
		return nil
	})
	if _, e := app.Test(httptest.NewRequest("GET", "/x?"+qs, nil)); e != nil {
		t.Fatalf("goi app: %v", e)
	}
	return got, err
}

func TestConversationFilterFromQuery(t *testing.T) {
	cases := []struct {
		name   string
		qs     string
		want   repository.ConversationFilter
		wantEr string // code loi mong doi, "" = khong loi
	}{
		{"khong co gi => khong loc", "", repository.ConversationFilter{}, ""},
		{"type group", "type=group", repository.ConversationFilter{Type: domain.ConversationGroup}, ""},
		{"type direct", "type=direct", repository.ConversationFilter{Type: domain.ConversationDirect}, ""},
		{"source cms", "source=cms", repository.ConversationFilter{Source: domain.SourceCMS}, ""},
		{"source local", "source=local", repository.ConversationFilter{Source: domain.SourceLocal}, ""},
		{"q", "q=Tet+2027", repository.ConversationFilter{Query: "Tet 2027"}, ""},
		{"gop het", "type=group&source=cms&q=a", repository.ConversationFilter{
			Type: domain.ConversationGroup, Source: domain.SourceCMS, Query: "a"}, ""},
		{"type la", "type=nhom", repository.ConversationFilter{}, "invalid_type"},
		{"source la", "source=sql", repository.ConversationFilter{}, "invalid_source"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseQuery(t, c.qs)

			if c.wantEr != "" {
				if err == nil {
					t.Fatalf("muon loi %s, khong co loi", c.wantEr)
				}
				if !strings.Contains(err.Error(), c.wantEr) {
					t.Fatalf("muon loi %s, nhan: %v", c.wantEr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("khong mong doi loi: %v", err)
			}
			if got != c.want {
				t.Errorf("= %+v, muon %+v", got, c.want)
			}
		})
	}
}
