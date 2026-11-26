package domain

import (
	"testing"

	"github.com/google/uuid"
)

func TestCanMentionAll(t *testing.T) {
	chatID := "wg_95f6fdbfaa17593e"
	cmsGroup := &Conversation{ID: uuid.New(), Source: SourceCMS, ExternalID: &chatID,
		CMSInfo: JSONMap{"botId": float64(1)}}
	localGroup := &Conversation{ID: uuid.New(), Source: SourceLocal}

	cases := []struct {
		name   string
		user   *User
		conv   *Conversation
		member *ConversationMember
		want   bool
	}{
		{"role admin", &User{Role: "admin"}, cmsGroup, nil, true},
		{"admin bot so huu nhom", &User{CMSProfile: JSONMap{"botAdminOf": []any{float64(1)}}}, cmsGroup, nil, true},
		{"admin bot khac", &User{CMSProfile: JSONMap{"botAdminOf": []any{float64(2)}}}, cmsGroup, nil, false},
		{"tro ly nhom", &User{CMSProfile: JSONMap{"assistantOf": []any{chatID}}}, cmsGroup, nil, true},
		{"tro ly nhom khac", &User{CMSProfile: JSONMap{"assistantOf": []any{"wg_khac"}}}, cmsGroup, nil, false},
		{"nguon assistant", &User{}, cmsGroup, &ConversationMember{Sources: JSONStrings{"assistant"}}, true},
		{"designer", &User{IsDesigner: true, CMSProfile: JSONMap{"webTeams": []any{map[string]any{"teamId": 27}}}},
			cmsGroup, &ConversationMember{Sources: JSONStrings{"team"}}, false},
		{"owner nhom CMS khong tinh", &User{}, cmsGroup, &ConversationMember{Role: MemberOwner}, false},
		{"owner nhom noi bo", &User{}, localGroup, &ConversationMember{Role: MemberOwner}, true},
		{"thanh vien nhom noi bo", &User{}, localGroup, &ConversationMember{Role: MemberNormal}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.user.CanMentionAll(tc.conv, tc.member); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNormalizeMentionInput(t *testing.T) {
	keys, all := NormalizeMentionInput([]string{" @u_55", "u_55", "", "@all", "u_9"})
	if !all || len(keys) != 2 || keys[0] != "u_55" || keys[1] != "u_9" {
		t.Fatalf("keys=%v all=%v", keys, all)
	}
}

func TestMentionKey(t *testing.T) {
	id := uuid.New()
	if k := (&User{ID: id, ActorID: "u_5"}).MentionKey(); k != "u_5" {
		t.Fatal(k)
	}
	if k := (&User{ID: id}).MentionKey(); k != id.String() {
		t.Fatal(k)
	}
}

func TestConversationBotDM(t *testing.T) {
	dm := &Conversation{CMSInfo: JSONMap{"type": "dm"}}
	group := &Conversation{CMSInfo: JSONMap{"type": "group"}}
	old := &Conversation{CMSInfo: JSONMap{"botId": float64(1)}} // truoc khi CMS gui type
	if !dm.IsBotDM() || dm.ClientType() != "dm" {
		t.Fatal("dm")
	}
	if group.IsBotDM() || group.ClientType() != "group" || old.IsBotDM() || old.ClientType() != "group" {
		t.Fatal("group")
	}
	if (&Conversation{}).IsBotDM() {
		t.Fatal("nhom noi bo")
	}
}
