package cmssync

import (
	"sort"
	"testing"

	"github.com/google/uuid"
)

func ids(n int) []uuid.UUID {
	out := make([]uuid.UUID, n)
	for i := range out {
		out[i] = uuid.New()
	}
	return out
}

func sortIDs(in []uuid.UUID) []string {
	out := make([]string, 0, len(in))
	for _, id := range in {
		out = append(out, id.String())
	}
	sort.Strings(out)
	return out
}

func eq(a, b []uuid.UUID) bool {
	sa, sb := sortIDs(a), sortIDs(b)
	if len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

// Day la cai quyet dinh CO ban event reload hay khong. Danh sach khong doi thi
// added/removed phai rong, neu khong ca nhom se nhan reload moi 60 giay.
func TestDiffKhongDoiThiRong(t *testing.T) {
	u := ids(4)

	cases := []struct {
		name          string
		before, after []uuid.UUID
		added, remove []uuid.UUID
	}{
		{"y nguyen", u, u, nil, nil},
		{"y nguyen nhung khac thu tu",
			[]uuid.UUID{u[0], u[1], u[2]},
			[]uuid.UUID{u[2], u[0], u[1]}, nil, nil},
		{"ca hai rong", nil, nil, nil, nil},
		{"them 1 nguoi",
			[]uuid.UUID{u[0], u[1]},
			[]uuid.UUID{u[0], u[1], u[2]},
			[]uuid.UUID{u[2]}, nil},
		{"bo 1 nguoi",
			[]uuid.UUID{u[0], u[1], u[2]},
			[]uuid.UUID{u[0], u[1]},
			nil, []uuid.UUID{u[2]}},
		{"thay nguoi (vua them vua bo)",
			[]uuid.UUID{u[0], u[1]},
			[]uuid.UUID{u[0], u[2]},
			[]uuid.UUID{u[2]}, []uuid.UUID{u[1]}},
		{"nhom moi, chua ai",
			nil, []uuid.UUID{u[0], u[1]},
			[]uuid.UUID{u[0], u[1]}, nil},
		{"go het thanh vien",
			[]uuid.UUID{u[0], u[1]}, nil,
			nil, []uuid.UUID{u[0], u[1]}},
		{"trung lap trong input khong sinh them",
			[]uuid.UUID{u[0], u[0], u[1]},
			[]uuid.UUID{u[1], u[0]}, nil, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			added, removed := diff(c.before, c.after)
			if !eq(added, c.added) {
				t.Errorf("added = %v, muon %v", sortIDs(added), sortIDs(c.added))
			}
			if !eq(removed, c.remove) {
				t.Errorf("removed = %v, muon %v", sortIDs(removed), sortIDs(c.remove))
			}

			banReload := len(added) > 0 || len(removed) > 0
			muonBan := len(c.added) > 0 || len(c.remove) > 0
			if banReload != muonBan {
				t.Errorf("ban reload = %v, muon %v", banReload, muonBan)
			}
		})
	}
}

// union dung cho nguoi nhan: ca nguoi dang o trong nhom va nguoi vua bi go.
func TestUnionGomCaHaiBenVaKhongTrung(t *testing.T) {
	u := ids(3)
	got := union([]uuid.UUID{u[0], u[1]}, []uuid.UUID{u[1], u[2]})

	if len(got) != 3 {
		t.Fatalf("union tra %d phan tu, muon 3 (khong trung): %v", len(got), sortIDs(got))
	}
	if !eq(got, u) {
		t.Errorf("union = %v, muon %v", sortIDs(got), sortIDs(u))
	}
}
