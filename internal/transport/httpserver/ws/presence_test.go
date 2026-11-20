package ws

import (
	"encoding/json"
	"testing"
)

// presence.list nhan ca 3 cach dat ten khoa cho id nhom, va de trong cung duoc.
func TestPresenceListPayloadNormalize(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"de trong", `{}`, ""},
		{"snake_case", `{"conversation_id":"wg_1"}`, "wg_1"},
		{"camelCase", `{"conversationId":"wg_2"}`, "wg_2"},
		{"groupId", `{"groupId":"wg_3"}`, "wg_3"},
		{"groupId thang khi co nhieu", `{"groupId":"wg_4","conversationId":"wg_5","conversation_id":"wg_6"}`, "wg_4"},
		{"camelCase thang snake_case", `{"conversationId":"wg_7","conversation_id":"wg_8"}`, "wg_7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p presenceListPayload
			if err := json.Unmarshal([]byte(c.body), &p); err != nil {
				t.Fatalf("parse %s: %v", c.body, err)
			}
			p.normalize()
			if p.ConversationID != c.want {
				t.Errorf("ConversationID = %q, muon %q", p.ConversationID, c.want)
			}
		})
	}
}

// Frame type phai dung ten trong hop dong — doi la vo FE.
func TestTenFrameType(t *testing.T) {
	want := map[string]string{
		InPing:          "ping",
		InMessageSend:   "message.send",
		InMessageRead:   "message.read",
		InMessageDelete: "message.delete",
		InTemplateSend:  "template.send",
		InTyping:        "typing",
		InPresenceList:  "presence.list",
	}
	if len(want) != 7 {
		t.Fatalf("dang co %d frame client gui len, cap nhat lai test + docs", len(want))
	}
	for got, exp := range want {
		if got != exp {
			t.Errorf("frame type = %q, muon %q", got, exp)
		}
	}
}
