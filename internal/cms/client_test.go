package cms

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abc-workspace/chat-service/internal/config"
)

const serviceKey = "mock_service_key"

// CMS gia lap de kiem: service key, chu ky HMAC va parse envelope.
func newCMS(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(config.CMS{
		Enabled:          true,
		BaseURL:          srv.URL,
		WorkspaceBaseURL: srv.URL + "/api/client/v1/workspace",
		WebhookURL:       srv.URL + "/webhook/web/designer",
		ServiceKey:       serviceKey,
		HTTPTimeout:      5 * time.Second,
		WebhookTimeout:   5 * time.Second,
	}), srv
}

func TestGroupsSendsServiceKey(t *testing.T) {
	var gotKey string
	c, _ := newCMS(t, func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Chat-Service-Key")
		_, _ = io.WriteString(w, `{"statusCode":200,"code":"SUCCESS","message":"Success","element":{"groups":[
			{"chatId":"wg_abc","name":"Nhom Thiet ke Web","botId":1,"orderTeamCodes":["DESIGN"],
			 "teams":[{"id":27,"code":"DESIGN","name":"Thiet ke"}],
			 "members":[{"cmsUserId":68,"openId":"local:x","name":"Tran Designer","sources":["team"],"teamCodes":["DESIGN"]}]}]}}`)
	})

	groups, err := c.Groups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != serviceKey {
		t.Errorf("service key = %q, want %q", gotKey, serviceKey)
	}
	if len(groups) != 1 || groups[0].ChatID != "wg_abc" {
		t.Fatalf("groups = %+v", groups)
	}
	m := groups[0].Members[0]
	if m.CMSUserID != 68 || m.Sources[0] != "team" || m.TeamCodes[0] != "DESIGN" {
		t.Errorf("member = %+v", m)
	}
}

func TestUsersMapsProfileAndAvatar(t *testing.T) {
	c, srv := newCMS(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ids"); got != "68" {
			t.Errorf("ids = %q", got)
		}
		_, _ = io.WriteString(w, `{"statusCode":200,"code":"SUCCESS","message":"Success","element":{"users":[
			{"cmsUserId":68,"openId":"local:x","name":"Tran Designer","username":"tran","email":"t@e.com",
			 "avatarUrl":"/api/avatars/68.png","status":"active","role":"user","isDesigner":true,
			 "webTeams":[{"botId":1,"teamId":27,"code":"DESIGN"}],"assistantOf":[],"botAdminOf":[]}]}}`)
	})

	users, err := c.UsersByCMSID(context.Background(), []int64{68})
	if err != nil {
		t.Fatal(err)
	}
	u := users[0]
	if want := srv.URL + "/api/avatars/68.png"; u.AvatarURL != want {
		t.Errorf("avatar = %q, want %q", u.AvatarURL, want)
	}
	if u.DisplayRole() != "designer" {
		t.Errorf("display role = %q, want designer", u.DisplayRole())
	}
	if u.CMSUserID == nil || *u.CMSUserID != 68 || u.OpenID != "local:x" {
		t.Errorf("identity sai: %+v", u)
	}
}

// Chu ky phai tinh tren dung bytes da gui, kem timestamp.
func TestSendEventSignature(t *testing.T) {
	var (
		gotSig  string
		gotTS   string
		gotBody []byte
	)
	c, _ := newCMS(t, func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Abc-Signature")
		gotTS = r.Header.Get("X-Abc-Timestamp")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"ok":true,"toast":{"type":"success","text":"Da gui yeu cau WEB-1"},"closeForm":true}`)
	})

	resp, err := c.SendEvent(context.Background(), EventFormSubmit, map[string]any{
		"formId": "create_task",
		"values": map[string]any{"title": "Banner"},
	}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Toast == nil || !resp.CloseForm {
		t.Fatalf("resp = %+v", resp)
	}
	if want := Sign(serviceKey, gotTS, gotBody); gotSig != want {
		t.Errorf("chu ky sai:\n got %s\nwant %s", gotSig, want)
	}

	var ev map[string]any
	if err := json.Unmarshal(gotBody, &ev); err != nil {
		t.Fatal(err)
	}
	if ev["schema"] != EventSchema || ev["type"] != EventFormSubmit {
		t.Errorf("khung event sai: %v", ev)
	}
	if id, _ := ev["eventId"].(string); len(id) < 5 {
		t.Errorf("thieu eventId: %v", ev["eventId"])
	}
}
