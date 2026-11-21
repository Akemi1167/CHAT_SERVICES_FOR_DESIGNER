package clientapi

import (
	"encoding/json"
	"testing"
)

// values cua action la JSON tuy y: split_task gui values.parts la MANG OBJECT.
// Neu ai doi Values thanh map[string]string thi request nay se bi invalid_body.
func TestActionReqValuesLongNhau(t *testing.T) {
	body := `{"action":"split_task","values":{"parts":[
      {"quantity":2,"brief":"Phan cua nguoi dang lam"},
      {"assigneeOpenId":"local:a","normItemId":"10","quantity":3,"brief":""}]}}`
	var req actionReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("values long nhau phai parse duoc: %v", err)
	}
	parts, ok := req.Values["parts"].([]any)
	if !ok || len(parts) != 2 {
		t.Fatalf("values.parts sai: %#v", req.Values["parts"])
	}
	second := parts[1].(map[string]any)
	if second["assigneeOpenId"] != "local:a" || second["quantity"] != float64(3) {
		t.Errorf("phan 2 sai: %v", second)
	}

	// Chuyen sang CMS phai y nguyen (khong ep kieu, khong lam phang).
	out, err := json.Marshal(map[string]any(req.Values))
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	_ = json.Unmarshal(out, &back)
	if len(back["parts"].([]any)) != 2 {
		t.Errorf("mat du lieu khi marshal lai: %s", out)
	}

	// Nut confirm (accept_task tren card phan): values {} hoac bo han deu hop le.
	for _, b := range []string{`{"action":"accept_task","values":{}}`, `{"action":"accept_task"}`} {
		var r actionReq
		if err := json.Unmarshal([]byte(b), &r); err != nil || r.Action != "accept_task" {
			t.Errorf("body %s: %v", b, err)
		}
	}
}
