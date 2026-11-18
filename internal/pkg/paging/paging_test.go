package paging

import (
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	if p := Normalize("", 0); p.Limit != DefaultLimit {
		t.Fatalf("limit = %d, want %d", p.Limit, DefaultLimit)
	}
	if p := Normalize("", 9999); p.Limit != MaxLimit {
		t.Fatalf("limit = %d, want %d", p.Limit, MaxLimit)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	c := DecodeCursor(EncodeCursor(now, "abc"))
	if !c.Valid || c.ID != "abc" || !c.CreatedAt.Equal(now) {
		t.Fatalf("cursor = %+v, want (%v, abc)", c, now)
	}
	if DecodeCursor("khong-phai-base64!!").Valid {
		t.Fatal("cursor rac phai tra Valid = false")
	}
}

func TestBuild(t *testing.T) {
	type row struct {
		at time.Time
		id string
	}
	key := func(r row) (time.Time, string) { return r.at, r.id }
	now := time.Now().UTC()

	p := Normalize("", 2)
	rows := []row{{now, "1"}, {now, "2"}, {now, "3"}} // repo tra limit+1
	items, info := Build(rows, p, key)
	if len(items) != 2 || !info.HasMore || info.NextCursor == "" {
		t.Fatalf("items = %d, info = %+v", len(items), info)
	}

	items, info = Build(rows[:1], p, key)
	if len(items) != 1 || info.HasMore {
		t.Fatalf("trang cuoi phai HasMore = false, got %+v", info)
	}
}
