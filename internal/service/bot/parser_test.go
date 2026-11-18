package bot

import "testing"

func TestParseMention(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		wantSlug string
		wantBody string
		wantOK   bool
	}{
		{"khong co mention", "xin chao ca nha", "", "", false},
		{"mention giua cau khong tinh", "hello @bot", "", "", false},
		{"mention don gian", "@bot", "bot", "", true},
		{"mention + mau", "@bot yeu_cau so_luong=3", "bot", "yeu_cau so_luong=3", true},
		{"co khoang trang dau", "  @bot-design help", "bot-design", "help", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, ok := ParseMention(tc.input)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if m.Slug != tc.wantSlug || m.Body != tc.wantBody {
				t.Fatalf("got (%q,%q), want (%q,%q)", m.Slug, m.Body, tc.wantSlug, tc.wantBody)
			}
		})
	}
}

func TestParseBody(t *testing.T) {
	t.Run("key=value", func(t *testing.T) {
		code, payload, _ := ParseBody(`yeu_cau tieu_de="Banner tet" so_luong=3 gap=true`)
		if code != "yeu_cau" {
			t.Fatalf("code = %q", code)
		}
		if payload["tieu_de"] != "Banner tet" {
			t.Fatalf("tieu_de = %v", payload["tieu_de"])
		}
		if payload["so_luong"] != int64(3) {
			t.Fatalf("so_luong = %v (%T)", payload["so_luong"], payload["so_luong"])
		}
		if payload["gap"] != true {
			t.Fatalf("gap = %v", payload["gap"])
		}
	})

	t.Run("json", func(t *testing.T) {
		code, payload, _ := ParseBody(`phieu_sua {"ma_task":"T-100","so_luong":2}`)
		if code != "phieu_sua" {
			t.Fatalf("code = %q", code)
		}
		if payload["ma_task"] != "T-100" {
			t.Fatalf("ma_task = %v", payload["ma_task"])
		}
	})

	t.Run("chi co ghi chu tu do", func(t *testing.T) {
		code, payload, _ := ParseBody(`yeu_cau lam gap cho toi nhe`)
		if code != "yeu_cau" {
			t.Fatalf("code = %q", code)
		}
		if payload["note"] != "lam gap cho toi nhe" {
			t.Fatalf("note = %v", payload["note"])
		}
	})
}
