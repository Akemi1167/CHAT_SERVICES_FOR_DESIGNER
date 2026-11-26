package message

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/domain"
)

func member(actorID, name string) domain.ConversationMember {
	u := &domain.User{ID: uuid.New(), ActorID: actorID, DisplayName: name}
	return domain.ConversationMember{UserID: u.ID, User: u}
}

func TestResolveMentions(t *testing.T) {
	sender := member("u_1", "Nguoi gui")
	des := member("u_55", "Tran Designer")
	tl := member("u_9", "Trinh Van Long")
	local := member("", "User dev") // chua co actorId -> khoa = UUID
	members := []domain.ConversationMember{sender, des, tl, local}

	t.Run("chi giu thanh vien, bo nguoi gui, bo trung, giu thu tu", func(t *testing.T) {
		keys, ids := resolveMentions(members, sender.UserID,
			[]string{"u_9", "u_1", "u_404", "u_55", "u_9"}, false)
		if got := strings.Join(keys, ","); got != "u_9,u_55" {
			t.Fatalf("keys = %s", got)
		}
		if len(ids) != 2 || ids[0] != tl.UserID || ids[1] != des.UserID {
			t.Fatalf("ids = %v", ids)
		}
	})

	t.Run("user khong co actorId nhac bang UUID", func(t *testing.T) {
		keys, _ := resolveMentions(members, sender.UserID, []string{local.UserID.String()}, false)
		if len(keys) != 1 || keys[0] != local.UserID.String() {
			t.Fatalf("keys = %v", keys)
		}
	})

	t.Run("nhac bang UUID cua user co actorId -> luu actorId", func(t *testing.T) {
		keys, _ := resolveMentions(members, sender.UserID, []string{des.UserID.String(), "u_55"}, false)
		if len(keys) != 1 || keys[0] != "u_55" {
			t.Fatalf("keys = %v", keys)
		}
	})

	t.Run("@all = moi thanh vien tru nguoi gui", func(t *testing.T) {
		keys, ids := resolveMentions(members, sender.UserID, nil, true)
		if len(keys) != 3 || len(ids) != 3 {
			t.Fatalf("keys = %v", keys)
		}
		for _, k := range keys {
			if k == "u_1" {
				t.Fatal("khong duoc nhac chinh nguoi gui")
			}
		}
	})

	t.Run("toi da MaxMentions", func(t *testing.T) {
		big := []domain.ConversationMember{sender}
		var keys []string
		for i := 0; i < domain.MaxMentions+20; i++ {
			m := member("", "x")
			big = append(big, m)
			keys = append(keys, m.UserID.String())
		}
		got, _ := resolveMentions(big, sender.UserID, keys, false)
		if len(got) != domain.MaxMentions {
			t.Fatalf("len = %d", len(got))
		}
	})
}

func TestMentionNotificationText(t *testing.T) {
	long := strings.Repeat("a", 100)
	cases := []struct{ name, sender, group, content, want string }{
		{"day du", "An", "Nhom Web", "Nho @Tran xem\n giup", "An nhắc đến bạn trong Nhom Web: Nho @Tran xem giup"},
		{"khong co noi dung", "An", "Nhom Web", "", "An nhắc đến bạn trong Nhom Web"},
		{"khong ten nhom", "An", "", "hi", "An nhắc đến bạn: hi"},
		{"cat 80 ky tu", "An", "G", long, "An nhắc đến bạn trong G: " + strings.Repeat("a", 80) + "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mentionNotificationText(tc.sender, tc.group, tc.content); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}
