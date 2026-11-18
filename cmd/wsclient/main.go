// Command wsclient la WebSocket client dong lenh de test chat realtime.
//
// Mo 2-3 cua so terminal, moi cua so 1 nguoi:
//
//	go run ./cmd/wsclient -sub an    -name An    -conv <room_id>
//	go run ./cmd/wsclient -sub binh  -name Binh  -conv <room_id>
//
// Go text roi Enter => gui tin nhan. Cac lenh:
//
//	/ping                 kiem tra ket noi
//	/typing               bao dang nhap
//	/read <message_id>    danh dau da doc
//	/json {...}           gui frame tho (bat ky frame nao trong docs/02-websocket.md)
//	/quit                 thoat
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/google/uuid"

	"github.com/abc-workspace/chat-service/internal/config"
	"github.com/abc-workspace/chat-service/internal/pkg/jwtauth"
)

func main() {
	addr := flag.String("addr", "ws://localhost:8080/ws", "dia chi WebSocket")
	token := flag.String("token", "", "JWT; de trong thi tu ky bang JWT_SECRET trong .env")
	sub := flag.String("sub", "wsclient", "external user id (claim sub) khi tu ky token")
	name := flag.String("name", "WS Client", "ten hien thi khi tu ky token")
	role := flag.String("role", "designer", "role khi tu ky token")
	conv := flag.String("conv", "", "conversation_id de gui tin nhan")
	flag.Parse()

	jwt := *token
	if jwt == "" {
		cfg, err := config.Load()
		if err != nil {
			exit("khong doc duoc config (.env): %v", err)
		}
		if jwt, err = jwtauth.Sign(cfg.Auth.JWTSecret, *sub, *name, *role, 24*time.Hour); err != nil {
			exit("khong ky duoc token: %v", err)
		}
	}

	conn, resp, err := websocket.DefaultDialer.Dial(*addr+"?token="+url.QueryEscape(jwt), nil)
	if err != nil {
		if resp != nil {
			exit("ket noi that bai (%s): %v", resp.Status, err)
		}
		exit("ket noi that bai: %v", err)
	}
	defer conn.Close()
	who := *name
	if *token != "" {
		who = "token duoc cung cap"
	}
	fmt.Printf("● da ket noi %s (%s). Go text roi Enter de gui, /quit de thoat.\n", *addr, who)

	// Mot goroutine doc, mot goroutine ghi: gorilla/websocket khong cho ghi dong thoi.
	go read(conn)

	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		frame, quit := build(line, *conv)
		if quit {
			fmt.Println("● ket thuc")
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
				time.Now().Add(time.Second))
			os.Exit(0)
		}
		if frame == "" {
			continue
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			exit("gui that bai: %v", err)
		}
	}
}

// build doi 1 dong input thanh frame JSON.
func build(line, conv string) (frame string, quit bool) {
	id := uuid.NewString()[:8]
	switch {
	case line == "/quit":
		return "", true
	case line == "/ping":
		return fmt.Sprintf(`{"id":%q,"type":"ping"}`, id), false
	case line == "/typing":
		return fmt.Sprintf(`{"id":%q,"type":"typing","payload":{"conversation_id":%q,"typing":true}}`, id, conv), false
	case strings.HasPrefix(line, "/read "):
		msgID := strings.TrimSpace(strings.TrimPrefix(line, "/read "))
		return fmt.Sprintf(`{"id":%q,"type":"message.read","payload":{"conversation_id":%q,"message_id":%q}}`,
			id, conv, msgID), false
	case strings.HasPrefix(line, "/json "):
		return strings.TrimPrefix(line, "/json "), false
	case strings.HasPrefix(line, "/"):
		fmt.Println("  lenh khong biet. Dung: /ping /typing /read <id> /json {...} /quit")
		return "", false
	}

	if conv == "" {
		fmt.Println("  thieu -conv <conversation_id> nen khong gui duoc tin nhan")
		return "", false
	}
	payload := map[string]any{
		"conversation_id": conv,
		"content":         line,
		"client_msg_id":   uuid.NewString(),
	}
	b, _ := json.Marshal(map[string]any{"id": id, "type": "message.send", "payload": payload})
	return string(b), false
}

// read in cac frame server day xuong, gon de doc tren terminal.
func read(conn *websocket.Conn) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("● mat ket noi:", err)
			os.Exit(0)
		}
		var f struct {
			Type    string          `json:"type"`
			Ref     string          `json:"ref"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			fmt.Println("←", string(raw))
			continue
		}

		var p struct {
			Content    string `json:"content"`
			SenderType string `json:"sender_type"`
			Sender     *struct {
				DisplayName string `json:"display_name"`
			} `json:"sender"`
			UserID  string                         `json:"user_id"`
			Typing  *bool                          `json:"typing"`
			Code    string                         `json:"code"`
			Message string                         `json:"message"`
			Task    *struct{ Code, Status string } `json:"task"`
		}
		_ = json.Unmarshal(f.Payload, &p)

		switch f.Type {
		case "message.new":
			who := p.SenderType
			if p.Sender != nil && p.Sender.DisplayName != "" {
				who = p.Sender.DisplayName
			}
			fmt.Printf("← %s: %s\n", who, firstLine(p.Content))
		case "typing":
			fmt.Printf("← %.8s dang nhap...\n", p.UserID)
		case "message.read":
			fmt.Printf("← %.8s da doc\n", p.UserID)
		case "error":
			fmt.Printf("← LOI %s: %s\n", p.Code, p.Message)
		case "task.created", "task.updated":
			if p.Task != nil {
				fmt.Printf("← %s: %s -> %s\n", f.Type, p.Task.Code, p.Task.Status)
				continue
			}
			fmt.Printf("← %s\n", f.Type)
		case "ack", "pong", "ready":
			fmt.Printf("← %s\n", f.Type)
		default:
			fmt.Printf("← %s %s\n", f.Type, truncate(string(f.Payload), 100))
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i] + " …"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func exit(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", args...)
	os.Exit(1)
}
