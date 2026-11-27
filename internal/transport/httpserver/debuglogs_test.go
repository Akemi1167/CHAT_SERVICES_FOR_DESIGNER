package httpserver

import (
	"testing"
	"time"
)

func TestParseWhen(t *testing.T) {
	if v, err := parseWhen(""); v != nil || err != nil {
		t.Fatal("rong = khong loc")
	}
	v, err := parseWhen("2026-11-27T10:00:00+07:00")
	if err != nil || v.UTC().Hour() != 3 {
		t.Fatalf("rfc3339: %v %v", v, err)
	}
	for in, back := range map[string]time.Duration{"15m": 15 * time.Minute, "2h": 2 * time.Hour, "7d": 7 * 24 * time.Hour} {
		v, err := parseWhen(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if d := time.Since(*v) - back; d < 0 || d > time.Second {
			t.Errorf("%s lech %v", in, d)
		}
	}
	if _, err := parseWhen("hom qua"); err == nil {
		t.Fatal("phai bao loi")
	}
}
