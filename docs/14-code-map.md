# 14 — Bản đồ code: từ URL tới hàm

Nhìn một URL như `{{base_url}}/api/client/v1/chat/auth/me` mà muốn biết "code nằm ở đâu".

## 1. Tra nhanh bằng lệnh

```bash
make route URL=/api/client/v1/chat/auth/me
# hoặc
./scripts/find-route.sh "{{base_url}}/api/client/v1/chat/auth/me"
```

In ra 3 tầng:

```
1) Khai bao route
   internal/transport/httpserver/clientapi/handler.go:43:  router.Get("/auth/me", h.Me)
2) Handler (ham xu ly)
   internal/transport/httpserver/clientapi/handler.go:58:  func (h *Handler) Me(c *fiber.Ctx) error
3) Service duoc goi (nghiep vu)
   h.convs.List
```

## 2. Đọc URL để biết đang ở "cửa" nào

Mỗi tiền tố là một tầng transport riêng, khai báo tập trung ở
[`internal/transport/httpserver/router.go`](../internal/transport/httpserver/router.go):

| Tiền tố URL | Ai gọi | Route khai báo tại | Handler nằm ở |
|---|---|---|---|
| `/api/v1/...` | Postman, CLI, service nội bộ | `router.go` (trong `registerRoutes`) | `internal/transport/httpserver/handler/*.go` |
| `/api/client/v1/chat/...` | **web chat (FE)** | `clientapi/handler.go` → `Register()` | `internal/transport/httpserver/clientapi/handler.go` |
| `/bot/v1/...` | **CMS gọi sang** | `botapi/handler.go` → `Register()` | `internal/transport/httpserver/botapi/handler.go` |
| `/ws` | FE (realtime) | `router.go` | `internal/transport/httpserver/ws/handler.go` |
| `/healthz`, `/readyz` | k8s/docker | `router.go` | ngay trong `router.go` |
| gRPC `chat.v1.*` | service khác | `grpcserver/server.go` | `grpcserver/*_handler.go` |

Mẹo đọc: **`/api/client/v1/chat` = bộ cho FE** (envelope `{statusCode, code, message, element}`,
field camelCase), **`/api/v1` = bộ gốc** (`{data, meta}`, snake_case). Cùng một nghiệp vụ, khác lớp vỏ.

## 3. Luồng đi của một request

```
HTTP  →  middleware (RequestID → AccessLog → Auth)      internal/transport/httpserver/middleware
      →  handler: đọc tham số, gọi service, map response
      →  service: nghiệp vụ + kiểm quyền                internal/service/<tên>
      →  repository: truy cập dữ liệu (interface)       internal/repository
      →  postgres: câu SQL/GORM thật                    internal/repository/postgres
      →  bảng + index                                   migrations/
```

Ví dụ đầy đủ với `GET /api/v1/messages/:message_id/replies`:

| Tầng | File |
|---|---|
| route | `internal/transport/httpserver/router.go` — `msg.Get("/:message_id/replies", msgH.Replies)` |
| handler | `internal/transport/httpserver/handler/message.go` — `func (h *MessageHandler) Replies` |
| service | `internal/service/message/service.go` — `func (s *Service) Replies` |
| repository (hợp đồng) | `internal/repository/repository.go` — `ListReplies`, `CountReplies` |
| repository (SQL) | `internal/repository/postgres/message_repo.go` |
| schema | `migrations/000001_init.up.sql` (bảng `messages`) |

## 4. Tự tra bằng grep (khi không nhớ lệnh)

```bash
# 1. Route: tìm đoạn cuối của URL trong tầng transport
grep -rn '"/auth/me"' internal/transport

# 2. Handler: tên hàm thấy ở bước 1
grep -rn "func (h \*Handler) Me(" internal/transport

# 3. Service: trong thân handler, mọi lời gọi nghiệp vụ đều dạng h.<service>.<Hàm>
grep -rn "func (s \*Service) Replies" internal/service

# 4. SQL: tìm trong repository postgres
grep -rn "func (r \*MessageRepo) ListReplies" internal/repository/postgres
```

Quy ước đặt tên giúp đoán nhanh: `h.msgs` → `internal/service/message`, `h.convs` →
`internal/service/conversation`, `h.cards` → `internal/service/card`, `h.tasks` →
`internal/service/task`, `h.bots` → `internal/service/bot`, `h.notifs` →
`internal/service/notification`, `h.sync` → `internal/service/cmssync`, `h.store` →
`internal/repository`.

## 5. Chiều ngược lại: từ code ra URL

```bash
# Hàm service này được gọi từ những API nào?
grep -rn "msgs.Replies\|\.Replies(" internal/transport
```

## 6. Thứ không nằm trong `router.go`

| Việc | Ở đâu |
|---|---|
| Frame WebSocket (`message.send`, `typing`…) | `internal/transport/httpserver/ws/handler.go` — hàm `dispatch`, mỗi loại frame là một `case` |
| gRPC method | `api/proto/chat/v1/*.proto` (hợp đồng) → `internal/transport/grpcserver/*_handler.go` |
| Event Chat gửi CMS | `internal/cms/client.go` (`SendEvent`), gọi từ `internal/service/card` và `internal/service/outbox` |
| Job chạy nền | `internal/app/app.go` — mục `g.Go(...)` trong `Run()` (hub, đồng bộ CMS, outbox) |
| Wiring (ai phụ thuộc ai) | `internal/app/app.go` — đọc file này là thấy toàn bộ sơ đồ |
