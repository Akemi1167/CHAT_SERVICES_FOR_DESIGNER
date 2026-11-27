// Command seeddemo dung san du lieu de test: 3 nguoi dung, 1 nhom, tin nhan,
// reply, thread va mot yeu cau qua @bot. In ra token + id + URL WebSocket
// de dan thang vao Postman hoac trinh duyet.
//
//	make seed                     # chay voi localhost:8080
//	make seed BASE=http://vps:8080
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/pkg/jwtauth"
)

type person struct {
	sub, name, role string
	token, userID   string
}

func main() {
	base := flag.String("base", "http://localhost:8080", "dia chi service")
	ttl := flag.Duration("ttl", 7*24*time.Hour, "thoi gian song cua token")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		exit("khong doc duoc cau hinh (.env): %v", err)
	}
	if cfg.Auth.JWTSecret == "" {
		exit("thieu JWT_SECRET: lenh nay chi dung cho moi truong dev/staging")
	}

	people := []*person{
		{sub: "demo-an", name: "An (Order)", role: "order"},
		{sub: "demo-binh", name: "Binh (Design)", role: "designer"},
		{sub: "demo-cuong", name: "Cuong (Design)", role: "designer"},
	}

	fmt.Println("Dang dung du lieu demo tai", *base, "...")
	for _, p := range people {
		p.token, err = jwtauth.Sign(cfg.Auth.JWTSecret, p.sub, p.name, p.role, *ttl)
		if err != nil {
			exit("ky token cho %s: %v", p.sub, err)
		}
		me, err := call(*base, "GET", "/api/v1/me", p.token, nil)
		if err != nil {
			exit("goi /me (%s): %v\n  -> service da chay chua? thu: make dev-up", p.name, err)
		}
		p.userID = str(data(me), "user_id")
	}
	an, binh, cuong := people[0], people[1], people[2]

	// 1) Nhom 3 nguoi
	conv, err := call(*base, "POST", "/api/v1/conversations", an.token, map[string]any{
		"type": "group", "name": "To Design (demo)",
		"member_user_ids": []string{binh.userID, cuong.userID},
	})
	if err != nil {
		exit("tao nhom: %v", err)
	}
	convID := str(data(conv), "id")

	// 2) Tin nhan goc + reply + reply long nhau (thanh 1 thread)
	root := send(*base, an, convID, map[string]any{
		"content": "Lam giup minh bo hinh su kien thang 11 nhe",
	})
	reply1 := send(*base, binh, convID, map[string]any{
		"content": "Minh nhan nhe", "reply_to_id": root, "thread_id": root,
	})
	send(*base, an, convID, map[string]any{
		"content": "OK, deadline 12h trua mai", "reply_to_id": reply1, "thread_id": root,
	})
	send(*base, binh, convID, map[string]any{
		"content": "Da gui ban nhap v1", "reply_to_id": root, "thread_id": root,
		"type": "image",
		"attachments": []map[string]any{{
			"kind": "image", "url": "https://picsum.photos/800/600", "name": "v1.png",
			"mime_type": "image/png", "size_bytes": 102400, "width": 800, "height": 600,
		}},
	})

	// 3) Mot yeu cau qua @bot (Chat chi luu mau; don/task do CMS so huu)
	send(*base, an, convID, map[string]any{
		"content": `@bot yeu_cau tieu_de="Bo hinh su kien thang 11" to=design so_luong=2 dinh_muc=A4`,
	})

	// 4) Danh dau da doc de co trang thai khac nhau giua cac nguoi
	_, _ = call(*base, "POST", "/api/v1/conversations/"+convID+"/read", binh.token,
		map[string]any{"message_id": root})

	wsBase := strings.Replace(strings.Replace(*base, "https://", "wss://", 1), "http://", "ws://", 1)

	fmt.Println()
	line()
	fmt.Println(" DU LIEU DEMO DA SAN SANG")
	line()
	fmt.Printf(" Nhom          : %s  (To Design (demo), 3 thanh vien)\n", convID)
	fmt.Printf(" Tin goc thread: %s  (co 3 phan hoi)\n", root)
	fmt.Printf(" Tin de reply  : %s\n", reply1)
	fmt.Println()
	for _, p := range people {
		fmt.Printf(" %-16s user_id=%s\n", p.name, p.userID)
		fmt.Printf("   token : %s\n", p.token)
		fmt.Printf("   ws    : %s/ws?token=%s\n\n", wsBase, p.token)
	}
	line()
	fmt.Println(" THU NHANH")
	line()
	fmt.Printf(" Lich su nhom:\n   curl -s '%s/api/v1/conversations/%s/messages?limit=20' \\\n     -H 'Authorization: Bearer <token>' | jq\n\n", *base, convID)
	fmt.Printf(" Phan hoi cua tin goc:\n   curl -s '%s/api/v1/messages/%s/replies?mode=thread' \\\n     -H 'Authorization: Bearer <token>' | jq\n\n", *base, root)
	fmt.Printf(" Chat tren terminal (2 cua so):\n   make ws SUB=demo-an   NAME=An   CONV=%s\n   make ws SUB=demo-binh NAME=Binh CONV=%s\n\n", convID, convID)
	fmt.Println(" Postman: import postman/chat-service.postman_collection.json + environment,")
	fmt.Println("   dan token vao bien 'token', conversation_id vao bien 'conversation_id'.")
}

func send(base string, p *person, convID string, body map[string]any) string {
	body["client_msg_id"] = fmt.Sprintf("seed-%d", time.Now().UnixNano())
	out, err := call(base, "POST", "/api/v1/conversations/"+convID+"/messages", p.token, body)
	if err != nil {
		exit("gui tin (%s): %v", p.name, err)
	}
	msg, _ := data(out)["message"].(map[string]any)
	return str(msg, "id")
}

func call(base, method, path, token string, body any) (map[string]any, error) {
	var reader io.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s -> %d: %.200s", method, path, resp.StatusCode, raw)
	}
	return out, nil
}

func data(m map[string]any) map[string]any {
	d, _ := m["data"].(map[string]any)
	return d
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func line() { fmt.Println(strings.Repeat("=", 72)) }

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
