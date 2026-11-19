package cmssync

import (
	"testing"

	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/cms"
	"github.com/abc-workspace/chat-service/internal/domain"
)

func userWith(cmsID int64, openID, name string) domain.User {
	u := domain.User{ID: uuid.New(), OpenID: openID, DisplayName: name}
	if cmsID > 0 {
		id := cmsID
		u.CMSUserID = &id
	}
	return u
}

// userIndex phai giu dung thu tu uu tien cua ham lookup cu:
// cmsUserId truoc, khong co moi den openId.
func TestUserIndexUuTienCMSUserIDTruoc(t *testing.T) {
	theoCMSID := userWith(68, "open-a", "theo cmsUserId")
	theoOpen := userWith(0, "open-a", "theo openId")

	ix := userIndex{
		byCMSID: map[int64]domain.User{68: theoCMSID},
		byOpen:  map[string]domain.User{"open-a": theoOpen},
	}

	cases := []struct {
		name string
		p    cms.Person
		want string // DisplayName mong doi, "" = khong tim thay
	}{
		{"co ca hai => lay theo cmsUserId", cms.Person{CMSUserID: 68, OpenID: "open-a"}, "theo cmsUserId"},
		{"chi co openId", cms.Person{OpenID: "open-a"}, "theo openId"},
		{"cmsUserId la => roi ve openId", cms.Person{CMSUserID: 999, OpenID: "open-a"}, "theo openId"},
		{"khong khop gi", cms.Person{CMSUserID: 999, OpenID: "open-z"}, ""},
		{"rong", cms.Person{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ix.get(c.p)
			switch {
			case c.want == "" && got != nil:
				t.Fatalf("muon khong tim thay, nhan duoc %q", got.DisplayName)
			case c.want != "" && got == nil:
				t.Fatalf("muon %q, nhan duoc nil", c.want)
			case c.want != "" && got.DisplayName != c.want:
				t.Fatalf("muon %q, nhan duoc %q", c.want, got.DisplayName)
			}
		})
	}
}

// get tra ve ban sao: sua ket qua khong duoc lam hong index dung chung.
func TestUserIndexGetTraBanSao(t *testing.T) {
	ix := userIndex{
		byCMSID: map[int64]domain.User{68: userWith(68, "open-a", "goc")},
		byOpen:  map[string]domain.User{},
	}
	u := ix.get(cms.Person{CMSUserID: 68})
	u.DisplayName = "da sua"

	if again := ix.get(cms.Person{CMSUserID: 68}); again.DisplayName != "goc" {
		t.Errorf("index bi sua theo: %q", again.DisplayName)
	}
}
