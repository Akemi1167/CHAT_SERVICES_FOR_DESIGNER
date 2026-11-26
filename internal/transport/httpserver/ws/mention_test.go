package ws

import (
	"encoding/json"
	"testing"
)

// message.send nhan mention o ca 2 dialect: /ws (snake_case) va /api/client/v1/chat/ws (camelCase).
func TestSendMessagePayloadMentions(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantAll bool
	}{
		{"internal snake_case", `{"conversation_id":"c","content":"hi","mentions":["u_55","u_9"],"mention_all":true}`, true},
		{"client camelCase", `{"groupId":"wg_1","text":"hi","mentions":["u_55","u_9"],"mentionAll":true}`, true},
		{"khong @all", `{"groupId":"wg_1","text":"hi","mentions":["u_55","u_9"]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var p sendMessagePayload
			if err := json.Unmarshal([]byte(tc.raw), &p); err != nil {
				t.Fatal(err)
			}
			p.normalize()
			if len(p.Mentions) != 2 || p.Mentions[0] != "u_55" {
				t.Fatalf("mentions = %v", p.Mentions)
			}
			if p.MentionAll != tc.wantAll {
				t.Fatalf("mentionAll = %v", p.MentionAll)
			}
		})
	}
}
