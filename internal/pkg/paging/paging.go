// Package paging chuan hoa cursor pagination cho timeline chat.
package paging

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultLimit = 30
	MaxLimit     = 100
)

type Page struct {
	Limit  int
	Cursor Cursor
}

// Cursor = (created_at, id) duoc encode base64 => on dinh khi co tin nhan cung timestamp.
type Cursor struct {
	CreatedAt time.Time
	ID        string
	Valid     bool
}

func Normalize(cursor string, limit int32) Page {
	l := int(limit)
	if l <= 0 {
		l = DefaultLimit
	}
	if l > MaxLimit {
		l = MaxLimit
	}
	return Page{Limit: l, Cursor: DecodeCursor(cursor)}
}

func EncodeCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%d|%s", createdAt.UTC().UnixMicro(), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func DecodeCursor(s string) Cursor {
	if s == "" {
		return Cursor{}
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return Cursor{}
	}
	var micro int64
	if _, err := fmt.Sscanf(parts[0], "%d", &micro); err != nil {
		return Cursor{}
	}
	return Cursor{CreatedAt: time.UnixMicro(micro).UTC(), ID: parts[1], Valid: true}
}

type PageInfo struct {
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

// Build cat bo phan tu "do them" (repo luon query limit+1) va sinh cursor ke tiep.
// key tra ve (created_at, id) cua phan tu => dung chung cho moi loai timeline.
func Build[T any](items []T, p Page, key func(T) (time.Time, string)) ([]T, PageInfo) {
	if len(items) <= p.Limit {
		return items, PageInfo{}
	}
	items = items[:p.Limit]
	last := items[len(items)-1]
	at, id := key(last)
	return items, PageInfo{NextCursor: EncodeCursor(at, id), HasMore: true}
}

// FetchLimit = so row repo can query (limit + 1 de biet con trang sau).
func (p Page) FetchLimit() int { return p.Limit + 1 }
