package cms

import (
	"encoding/json"
	"testing"
)

func TestGroupAvatarURL(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantSet bool
		want    string
	}{
		{"khong gui -> giu nguyen", `{"chatId":"wg_1"}`, false, ""},
		{"null -> go avatar", `{"chatId":"wg_1","avatarUrl":null}`, true, ""},
		{"rong -> go avatar", `{"chatId":"wg_1","avatarUrl":""}`, true, ""},
		{"co gia tri", `{"chatId":"wg_1","avatarUrl":"/api/avatars/g1.png"}`, true, "/api/avatars/g1.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var g Group
			if err := json.Unmarshal([]byte(tc.raw), &g); err != nil {
				t.Fatal(err)
			}
			if g.AvatarURL.Set != tc.wantSet || g.AvatarURL.Value != tc.want {
				t.Fatalf("got %+v", g.AvatarURL)
			}
		})
	}
}

func TestGroupInfoType(t *testing.T) {
	for raw, want := range map[string]string{"": "group", "group": "group", "dm": "dm", " DM ": "dm", "la": "group"} {
		if got := (Group{Type: raw}).Info()["type"]; got != want {
			t.Errorf("type %q -> %v, want %s", raw, got, want)
		}
	}
}
