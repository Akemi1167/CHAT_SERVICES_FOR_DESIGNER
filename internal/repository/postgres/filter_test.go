package postgres

import "testing"

// Nguoi dung go "%" hay "_" trong o tim kiem khong duoc tro thanh ky tu dai dien
// cua LIKE (neu khong "%" se quet ca bang).
func TestEscapeLike(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Tet", "Tet"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`c\d`, `c\\d`},
		{`%_\`, `\%\_\\`},
		{"", ""},
	}
	for _, c := range cases {
		if got := escapeLike(c.in); got != c.want {
			t.Errorf("escapeLike(%q) = %q, muon %q", c.in, got, c.want)
		}
	}
}
