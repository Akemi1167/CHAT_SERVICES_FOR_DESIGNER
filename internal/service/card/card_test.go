package card

import (
	"encoding/json"
	"testing"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// Card that do CMS gui (rut gon): nut "Nhan task" chi hien voi nguoi duoc giao,
// nut "Huy" chi hien voi order.
const sample = `{
  "schema": "abc.card/v1",
  "header": {"code": "WEB-20261118-0006", "title": "Banner Tet 2027",
             "status": {"text": "Chua nhan", "tone": "warning"}},
  "visibleTo": null,
  "elements": [
    {"type": "field", "label": "To", "value": "Thiet ke"},
    {"type": "button", "text": "Nhan task", "action": "accept_task", "visibleTo": ["u_68"]},
    {"type": "button", "text": "Huy", "action": "cancel_task", "visibleTo": ["u_67"]},
    {"type": "button", "text": "Xem don", "action": "open_order"}
  ]
}`

func parse(t *testing.T) domain.JSONMap {
	t.Helper()
	var c domain.JSONMap
	if err := json.Unmarshal([]byte(sample), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestFilterForViewer(t *testing.T) {
	c := parse(t)

	cases := map[string][]string{
		"u_68": {"accept_task", "open_order"}, // designer duoc giao
		"u_67": {"cancel_task", "open_order"}, // order tao don
		"u_99": {"open_order"},                // nguoi ngoai cuoc
	}
	for actor, want := range cases {
		got := Actions(c, actor)
		if len(got) != len(want) {
			t.Fatalf("%s: actions = %v, want %v", actor, got, want)
		}
		seen := map[string]bool{}
		for _, a := range got {
			seen[a] = true
		}
		for _, w := range want {
			if !seen[w] {
				t.Errorf("%s: thieu action %q (got %v)", actor, w, got)
			}
		}
	}

	// Loc khong duoc lam hong phan con lai cua card.
	filtered := FilterForViewer(c, "u_99")
	header, _ := filtered["header"].(map[string]any)
	if header["code"] != "WEB-20261118-0006" {
		t.Errorf("header bi mat sau khi loc: %v", filtered["header"])
	}
	if _, ok := filtered["elements"].([]any); !ok {
		t.Errorf("elements phai con lai dang mang: %T", filtered["elements"])
	}
}

func TestCanAct(t *testing.T) {
	c := parse(t)
	cases := []struct {
		actor, action string
		want          bool
	}{
		{"u_68", "accept_task", true},
		{"u_67", "accept_task", false}, // khong nam trong visibleTo cua nut
		{"u_99", "cancel_task", false},
		{"u_99", "open_order", true},    // nut khong gioi han
		{"u_68", "khong_co_nut", false}, // action khong ton tai trong card
	}
	for _, tc := range cases {
		if got := CanAct(c, tc.action, tc.actor); got != tc.want {
			t.Errorf("CanAct(%s, %s) = %v, want %v", tc.actor, tc.action, got, tc.want)
		}
	}
}

// Card gioi han o cap cao nhat thi khong ai ngoai danh sach duoc thay.
func TestCardLevelVisibleTo(t *testing.T) {
	c := domain.JSONMap{
		"schema":    "abc.card/v1",
		"visibleTo": []any{"u_1"},
		"elements":  []any{map[string]any{"action": "x"}},
	}
	if FilterForViewer(c, "u_2") != nil {
		t.Error("card rieng tu khong duoc hien cho nguoi khac")
	}
	if FilterForViewer(c, "u_1") == nil {
		t.Error("nguoi trong visibleTo phai thay card")
	}
}
