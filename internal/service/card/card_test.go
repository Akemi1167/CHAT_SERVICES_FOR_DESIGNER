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

// Card that cua CMS sau dot cap nhat 20/11/2026: nut nam trong
// {"type":"actions","items":[...]}, co them phan tu parts / links, form co
// rows va truong kieu parts. Chat khong hieu may thu nay — chi duoc bo DUNG
// nhung nut ma nguoi xem khong nam trong visibleTo.
const cmsCard = `{
  "schema": "abc.card/v1",
  "header": {"code": "WEB-20261120-0003", "title": "Banner Tet",
             "status": {"text": "Da chia", "tone": "info"}, "note": null},
  "elements": [
    {"type": "parts", "title": "Cac phan", "items": [
      {"index": 1, "total": 2, "code": "WEB-20261120-0003-P1", "messageId": "m1",
       "assignee": {"actor": "u_2"}, "assigneeName": "An",
       "status": {"text": "Dang lam", "tone": "info"}},
      {"index": 2, "total": 2, "code": "WEB-20261120-0003-P2", "messageId": "m2",
       "assignee": {"actor": "u_6"}, "assigneeName": "Binh",
       "status": {"text": "Chua nhan", "tone": "warning"}}
    ]},
    {"type": "links", "title": "Drive", "items": [
      {"label": "Folder don", "url": "https://drive.example/order"}
    ]},
    {"type": "actions", "items": [
      {"action": "accept_task", "label": "Nhan task", "placement": "primary",
       "visibleTo": ["u_6"], "confirm": {"title": "Nhan phan 2/2?"}},
      {"action": "split_task", "label": "Chia task", "placement": "menu",
       "visibleTo": ["u_1"],
       "form": {
         "rows": [{"label": "Nguoi hien tai", "value": "An"}],
         "fields": [{"type": "parts", "name": "parts", "total": 6, "unit": "banner",
                     "minParts": 2, "maxParts": 5, "first": {"quantity": 2},
                     "assigneeOptions": [{"value": "local:a", "label": "An"}],
                     "normOptions": [{"value": "10", "label": "A4"}],
                     "defaultNormItemId": "10", "default": null}]
       }},
      {"action": "sync_delivery_drive", "label": "Dong bo Drive",
       "visibleTo": ["u_6", "u_1"]},
      {"action": "open_order", "label": "Xem don"}
    ]}
  ]
}`

func parseCMS(t *testing.T) domain.JSONMap {
	t.Helper()
	var c domain.JSONMap
	if err := json.Unmarshal([]byte(cmsCard), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func elementByType(t *testing.T, c domain.JSONMap, typ string) map[string]any {
	t.Helper()
	for _, e := range c["elements"].([]any) {
		if m, ok := e.(map[string]any); ok && m["type"] == typ {
			return m
		}
	}
	t.Fatalf("mat phan tu %q sau khi loc", typ)
	return nil
}

// Nut trong actions.items[] duoc loc theo tung nguoi; action bi an thi khong bam duoc.
func TestCMSActionsItems(t *testing.T) {
	c := parseCMS(t)
	cases := map[string][]string{
		"u_6": {"accept_task", "sync_delivery_drive", "open_order"}, // nguoi nhan phan 2
		"u_1": {"split_task", "sync_delivery_drive", "open_order"},  // admin
		"u_2": {"open_order"},                                       // nguoi lam phan 1
	}
	all := []string{"accept_task", "split_task", "sync_delivery_drive", "open_order"}
	for actor, want := range cases {
		allowed := map[string]bool{}
		for _, a := range want {
			allowed[a] = true
		}
		if got := Actions(c, actor); len(got) != len(want) {
			t.Errorf("%s: actions = %v, muon %v", actor, got, want)
		}
		for _, a := range all {
			if got := CanAct(c, a, actor); got != allowed[a] {
				t.Errorf("CanAct(%s, %s) = %v, muon %v", actor, a, got, allowed[a])
			}
		}
		items := elementByType(t, FilterForViewer(c, actor), "actions")["items"].([]any)
		if len(items) != len(want) {
			t.Errorf("%s: con %d nut, muon %d", actor, len(items), len(want))
		}
	}
}

// Phan tu / khoa la va gia tri null phai con nguyen sau khi loc.
func TestFilterGiuNguyenPhanTuLa(t *testing.T) {
	c := parseCMS(t)

	// u_2 khong nam trong visibleTo nao, nhung parts.items[].assignee.actor chi
	// la du lieu hien thi: ca 2 phan (ke ca phan cua nguoi khac) phai con.
	got := FilterForViewer(c, "u_2")
	parts := elementByType(t, got, "parts")["items"].([]any)
	if len(parts) != 2 {
		t.Fatalf("parts.items con %d, muon 2", len(parts))
	}
	p2 := parts[1].(map[string]any)
	if p2["messageId"] != "m2" || p2["assignee"].(map[string]any)["actor"] != "u_6" {
		t.Errorf("phan 2 bi sua: %v", p2)
	}
	links := elementByType(t, got, "links")["items"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["url"] != "https://drive.example/order" {
		t.Errorf("links bi sua: %v", links)
	}

	// Khoa co gia tri null khong duoc roi.
	header := got["header"].(map[string]any)
	if v, ok := header["note"]; !ok || v != nil {
		t.Errorf("header.note (null) bi roi: %v", header)
	}

	// Form cua nut split_task (admin thay): rows + truong kieu parts con nguyen.
	admin := FilterForViewer(c, "u_1")
	node, ok := FindAction(admin, "split_task")
	if !ok {
		t.Fatal("admin phai thay split_task")
	}
	form := node["form"].(map[string]any)
	if rows := form["rows"].([]any); len(rows) != 1 {
		t.Errorf("form.rows bi mat: %v", form["rows"])
	}
	field := form["fields"].([]any)[0].(map[string]any)
	for _, k := range []string{"total", "unit", "minParts", "maxParts", "first",
		"assigneeOptions", "normOptions", "defaultNormItemId", "default"} {
		if _, ok := field[k]; !ok {
			t.Errorf("truong parts mat khoa %q", k)
		}
	}

	// Loc xong phai giong het ban goc ngoai nhung nut bi an: so voi ban goc da
	// bo tay 2 nut u_2 khong thay.
	want := parseCMS(t)
	actions := elementByType(t, want, "actions")
	actions["items"] = actions["items"].([]any)[3:]
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Errorf("card sau loc lech ban goc:\n got %s\nwant %s", a, b)
	}

	// Khong duoc sua card goc.
	if n := len(elementByType(t, c, "actions")["items"].([]any)); n != 4 {
		t.Errorf("card goc bi sua, con %d nut", n)
	}
}
