package clientview

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func sampleUser() *domain.User {
	id := int64(6)
	return &domain.User{
		ID:          uuid.New(),
		ActorID:     "u_6",
		OpenID:      "local:abc",
		CMSUserID:   &id,
		ExternalID:  "local:abc",
		Username:    "tester",
		DisplayName: "tester",
		Email:       "a@b.c",
		Role:        "designer",
		Status:      "active",
		IsDesigner:  true,
		Presence:    domain.PresenceOnline,
		CMSProfile:  domain.JSONMap{"webTeams": []any{}},
	}
}

// Be mat client KHONG duoc phoi truong noi bo cua Chat ra cho UI.
func TestUserKhongPhoiTruongNoiBo(t *testing.T) {
	got := User(sampleUser())

	for _, k := range []string{"actorId", "name", "cmsUserId", "openId", "presence", "profile"} {
		if _, ok := got[k]; !ok {
			t.Errorf("thieu khoa %q", k)
		}
	}
	// Day la nhung truong chi Chat dung, khong co trong hop dong CMS.
	for _, k := range []string{
		"externalId", "external_id", "status", "syncedAt", "synced_at",
		"isDesigner", "is_designer", "lastSeenAt", "last_seen_at",
		"createdAt", "updatedAt", "displayName", "cmsProfile",
	} {
		if _, ok := got[k]; ok {
			t.Errorf("khong duoc phoi khoa %q", k)
		}
	}
}

// REST va WebSocket phai cho ra DUNG mot hinh dang: cung goi Message().
func TestMessageDungTenTruongTheoHopDong(t *testing.T) {
	m := &domain.Message{
		ID:             uuid.New(),
		ConversationID: uuid.New(),
		Type:           domain.MessageText,
		SenderType:     domain.SenderUser,
		Content:        "xin chao",
		ClientMsgID:    "c-1",
		CreatedAt:      time.Now().UTC(),
		Sender:         sampleUser(),
	}
	got := Message(m, "u_6")

	if got["text"] != "xin chao" {
		t.Errorf(`"text" = %v, muon "xin chao" (KHONG duoc dung ten "content")`, got["text"])
	}
	if _, ok := got["content"]; ok {
		t.Error(`khong duoc co khoa "content" — hop dong dung "text"`)
	}
	if got["conversationId"] == nil {
		t.Error(`thieu "conversationId"`)
	}
	if _, ok := got["conversation_id"]; ok {
		t.Error("con khoa snake_case")
	}
}

// Payload: cung mot event, du payload da di qua broker hay chua, phai cho ra
// hinh dang nhu nhau.
func TestPayloadVoiMessage(t *testing.T) {
	m := domain.Message{
		ID: uuid.New(), ConversationID: uuid.New(),
		Type: domain.MessageText, SenderType: domain.SenderUser,
		Content: "abc", CreatedAt: time.Now().UTC(),
	}

	for _, in := range []any{&m, m} {
		out, ok := Payload(in, "u_6").(map[string]any)
		if !ok {
			t.Fatalf("Payload tra %T, muon map", Payload(in, "u_6"))
		}
		if out["text"] != "abc" {
			t.Errorf(`"text" = %v`, out["text"])
		}
	}
}

// Payload dang map (read, typing, presence, reload) chi can doi ten khoa.
func TestPayloadDoiKhoaSnakeSangCamel(t *testing.T) {
	in := map[string]any{
		"conversation_id": "c1",
		"message_id":      "m1",
		"user_id":         "u1",
		"seen_at":         "now",
		"typing":          true,
		"nested":          map[string]any{"actor_id": "u_6"},
		"list":            []any{map[string]any{"chat_id": "wg_1"}},
	}
	out := Payload(in, "u_6").(map[string]any)

	for _, k := range []string{"conversationId", "messageId", "userId", "seenAt", "typing"} {
		if _, ok := out[k]; !ok {
			t.Errorf("thieu %q, co: %v", k, keys(out))
		}
	}
	if _, ok := out["conversation_id"]; ok {
		t.Error("con khoa snake_case")
	}
	if out["nested"].(map[string]any)["actorId"] != "u_6" {
		t.Error("map long nhau chua duoc doi")
	}
	if out["list"].([]any)[0].(map[string]any)["chatId"] != "wg_1" {
		t.Error("map trong slice chua duoc doi")
	}
}

func TestCamelGiuNguyenKhoaKhongCoGachDuoi(t *testing.T) {
	cases := map[string]string{
		"typing":          "typing",
		"conversation_id": "conversationId",
		"a_b_c":           "aBC",
		"id":              "id",
		"":                "",
	}
	for in, want := range cases {
		if got := camel(in); got != want {
			t.Errorf("camel(%q) = %q, muon %q", in, got, want)
		}
	}
}

// Notification: chi phoi truong UI can.
func TestNotification(t *testing.T) {
	n := &domain.Notification{
		ID: uuid.New(), UserID: uuid.New(),
		Text: "co viec moi", Read: false, CreatedAt: time.Now().UTC(),
	}
	got := Notification(n)
	if got["text"] != "co viec moi" || got["read"] != false {
		t.Errorf("sai noi dung: %v", got)
	}
	if _, ok := got["userId"]; ok {
		t.Error("khong can tra userId — thong bao von chi gui cho chinh nguoi do")
	}
	if _, err := json.Marshal(got); err != nil {
		t.Errorf("khong marshal duoc: %v", err)
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
