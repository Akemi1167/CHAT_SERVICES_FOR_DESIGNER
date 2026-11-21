package botapi

import (
	"encoding/json"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/abc-workspace/chat-service/internal/pkg/apperr"
)

// Body cu (chi card) phai parse duoc y nhu truoc — CMS khong gui thread/notify
// thi hai field do la nil/rong.
func TestPatchMessageReqTuongThichNguoc(t *testing.T) {
	var req patchMessageReq
	body := `{"card":{"schema":"abc.card/v1"},"cardVersion":3}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("parse body cu: %v", err)
	}
	if req.CardVersion != 3 || req.Card == nil {
		t.Errorf("mat du lieu card: %+v", req)
	}
	if req.Thread != nil {
		t.Error("khong gui thread thi phai la nil")
	}
	if len(req.Notify) != 0 {
		t.Error("khong gui notify thi phai rong")
	}
}

// Body gop: doc duoc ca 3 phan.
func TestPatchMessageReqDocDuPhanGop(t *testing.T) {
	body := `{
      "card": {"schema":"abc.card/v1"},
      "cardVersion": 4,
      "thread": {"text":"da nop bai","bot":{"id":1,"name":"Bot"}},
      "notify": [
        {"actorId":"u_6","text":"thong bao 1"},
        {"actorId":"u_7","text":"thong bao 2","messageId":"abc"}
      ]
    }`
	var req patchMessageReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("parse body gop: %v", err)
	}

	if req.Thread == nil || req.Thread.Text != "da nop bai" {
		t.Fatalf("thread sai: %+v", req.Thread)
	}
	if req.Thread.Bot == nil || req.Thread.Bot.Name != "Bot" {
		t.Errorf("thread.bot sai: %+v", req.Thread.Bot)
	}
	if len(req.Notify) != 2 {
		t.Fatalf("muon 2 notify, nhan %d", len(req.Notify))
	}
	if req.Notify[0].ActorID != "u_6" || req.Notify[0].Text != "thong bao 1" {
		t.Errorf("notify[0] sai: %+v", req.Notify[0])
	}
	// messageId bo trong => handler tu dien id cua card.
	if req.Notify[0].MessageID != "" {
		t.Errorf("notify[0].messageId phai rong, nhan %q", req.Notify[0].MessageID)
	}
	if req.Notify[1].MessageID != "abc" {
		t.Errorf("notify[1].messageId = %q", req.Notify[1].MessageID)
	}
}

// partError dung cho loi cua tung phan phu: cung hinh dang voi loi chung
// ({code, message}) de CMS chi phai doc mot kieu.
func TestPartError(t *testing.T) {
	got := partError(apperr.Invalid("text_required", "Thieu text"))

	// Gia tri ben trong la fiber.Map (kieu CO TEN cua map[string]any) nen phai
	// ep dung kieu do — ep sang map[string]any se that bai.
	e, ok := got["error"].(fiber.Map)
	if !ok {
		t.Fatalf("khong co khoa error dang fiber.Map: %#v", got)
	}
	if e["code"] != "text_required" {
		t.Errorf(`code = %v, muon "text_required"`, e["code"])
	}
	if e["message"] != "Thieu text" {
		t.Errorf("message = %v", e["message"])
	}

	// Phai marshal duoc de tra ve cho CMS.
	if _, err := json.Marshal(got); err != nil {
		t.Errorf("khong marshal duoc: %v", err)
	}
}

// Loi khong phai apperr van ra duoc {code, message} chu khong panic.
func TestPartErrorVoiLoiLa(t *testing.T) {
	got := partError(errLa{})
	if _, err := json.Marshal(got); err != nil {
		t.Fatalf("khong marshal duoc: %v", err)
	}
	if got["error"] == nil {
		t.Error("thieu khoa error")
	}
}

type errLa struct{}

func (errLa) Error() string { return "loi la" }

// POST /messages/:id/thread nhan card (chia task: card cua phan 2..N nam trong
// thread card goc). Body chi co text nhu truoc van parse binh thuong.
func TestThreadReqNhanCard(t *testing.T) {
	body := `{"bot":{"id":1,"name":"Designer Bot"},
      "card":{"schema":"abc.card/v1",
              "header":{"code":"WEB-20261120-0003-P2","title":"Banner Tet · Phan 2/3"},
              "elements":[{"type":"actions","items":[
                {"action":"accept_task","label":"Nhan task","visibleTo":["u_6"]}]}]}}`
	var req threadReq
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("parse body co card: %v", err)
	}
	if req.Card == nil || req.Card["schema"] != "abc.card/v1" {
		t.Fatalf("mat card: %+v", req)
	}
	if req.Text != "" || req.CardVersion != 0 {
		t.Errorf("khong gui text/cardVersion thi phai rong: %+v", req)
	}

	var old threadReq
	if err := json.Unmarshal([]byte(`{"text":"da nhan task"}`), &old); err != nil {
		t.Fatalf("parse body cu: %v", err)
	}
	if old.Text != "da nhan task" || old.Card != nil {
		t.Errorf("body cu sai: %+v", old)
	}
}
