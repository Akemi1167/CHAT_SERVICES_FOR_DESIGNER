# Source — kiến trúc & cách sửa code

Dành cho người sửa code của **chính service này**: tầng nào làm gì, một URL dẫn tới hàm
nào, và checklist khi thêm/sửa/xoá field.

Gọi API thế nào → [backend.md](backend.md) · Làm FE → [frontend.md](frontend.md) ·
Đưa lên server → [deploy.md](deploy.md)

## Nội dung

- [Kiến trúc](#kiến-trúc)
- [Bản đồ code: từ URL tới hàm](#bản-đồ-code-từ-url-tới-hàm)
- [Thêm / sửa / xoá field & API (checklist)](#thêm--sửa--xoá-field--api-checklist)
- [Phần còn lại của spec map vào kiến trúc](#phần-còn-lại-của-spec-map-vào-kiến-trúc)

---

## Kiến trúc

### 1. Tầng và trách nhiệm

```
            ┌──────────────────────────────────────────────┐
  FE (web)  │  REST  /api/v1/...        WebSocket  /ws     │
            └──────────────┬───────────────────┬───────────┘
                           │                   │
   service khác ──gRPC──▶  │                   │
                           ▼                   ▼
              ┌───────────────────────────────────────────┐
  transport   │ httpserver (Fiber) · ws · grpcserver      │  parse input, auth,
              │ middleware, mapper domain<->proto/JSON    │  map lỗi -> status code
              └──────────────────┬────────────────────────┘
                                 ▼
              ┌───────────────────────────────────────────┐
  service     │ auth · conversation · message · bot       │  TOÀN BỘ nghiệp vụ,
              │ (kiểm tra quyền, transaction, publish ev) │  không biết HTTP/gRPC
              └──────────────────┬────────────────────────┘
                                 ▼
              ┌───────────────────────────────────────────┐
  repository  │ interface (port)  →  postgres/gorm        │  chỉ truy cập dữ liệu
              └──────────────────┬────────────────────────┘
                                 ▼
                          PostgreSQL

              realtime.Hub ──Broker(memory|redis)──▶ các instance khác
```

Quy tắc: **phụ thuộc chỉ chảy từ ngoài vào trong.** `domain` không import bất kỳ package nào của app;
`service` chỉ biết interface của `repository`; `transport` chỉ gọi `service`.

Lợi ích thực tế:
- Thêm transport mới (ví dụ GraphQL, Kafka consumer) không phải sửa nghiệp vụ.
- Test service bằng mock repository, không cần DB.
- Đổi DB/driver chỉ sửa `internal/repository/postgres`.

### 2. Luồng gửi 1 tin nhắn (quan trọng nhất)

```
FE --(WS frame message.send)--> ws.Handler
   ws.Handler: parse frame, lấy Identity từ context (đã auth lúc handshake)
      └─▶ message.Service.Send()
            1. conversation.EnsureMember()        -> chặn người ngoài nhóm
            2. kiểm tra client_msg_id             -> idempotent, retry không tạo tin trùng
            3. validate type/content/thread/reply
            4. phát hiện "@bot" trong nội dung    -> type = bot_command
            5. TRANSACTION: insert messages + update conversations.last_message_*
            6. hub.Publish(message.new, recipients = members)
            7. nếu có bot: bot.Service.Dispatch() (NGOÀI transaction)
                 - TemplateHandler: tìm mẫu, validate field required, INSERT bot_submissions
                 - trả về ReplyText
               rồi insert tin nhắn trả lời của bot (sender_type = bot) + publish
            8. trả kết quả -> ws gửi frame "ack" cho đúng client đã gửi
   hub: nhận event từ broker -> đẩy xuống mọi client của từng recipient
```

Vì sao bot chạy **ngoài** transaction: bot lỗi (sai mẫu, thiếu field) thì tin nhắn của người dùng
vẫn phải còn trong hội thoại, và bot chỉ trả lời "thiếu trường X" — không rollback tin nhắn.

### 3. Realtime và scale nhiều instance

- `realtime.Hub` giữ map `user_id -> các kết nối WS` **của instance hiện tại**.
- Mỗi event mang sẵn danh sách `recipients` (user id). Service tính recipients từ thành viên hội thoại.
- `Broker` quyết định event đi tới instance nào:
  - `MemoryBroker` — 1 instance, dùng khi dev.
  - `RedisBroker` — Pub/Sub, mọi instance đều nhận, instance nào có client thì đẩy xuống. Bật bằng `REDIS_ENABLED=true`.
- Nhờ vậy **không cần sticky session** ở load balancer (chỉ cần LB hỗ trợ WebSocket upgrade).

Lưu ý: Redis Pub/Sub là *at-most-once*. Khi FE reconnect, hãy gọi
`GET /api/v1/conversations/:id/messages?cursor=...` để đồng bộ phần đã bỏ lỡ — đây là lý do API
dùng cursor `(created_at, id)` thay vì offset.

Mỗi client có buffer gửi (`WS_SEND_BUFFER`). Client quá chậm làm đầy buffer sẽ bị đóng kết nối
để không kéo chậm cả hub — FE tự reconnect rồi resync.

### 4. Những quyết định thiết kế đáng chú ý

| Vấn đề | Cách làm | Lý do |
|---|---|---|
| Trùng tin nhắn khi FE retry | `client_msg_id` do FE sinh + unique index `(conversation_id, client_msg_id)` | WS mất kết nối giữa lúc gửi là chuyện thường |
| Chat riêng bị tạo trùng | `direct_key = sha1(sorted(2 user id))` + unique index | 2 người luôn chỉ có 1 hội thoại |
| Đếm chưa đọc | `conversation_members.last_read_at` + `COUNT(*)` theo `created_at >` | nhanh, không cần bảng trạng thái từng tin nhắn |
| "Ai đã xem tin này" | bảng `message_receipts` | hiển thị danh sách người đã đọc theo spec |
| Thread theo nhiệm vụ | `messages.thread_id` trỏ về message gốc | phản hồi hiện cả trong nhóm và trong thread (query theo `thread_id` hoặc `thread_id IS NULL`) |
| Field "mềm" hay đổi | cột `metadata jsonb` ở messages/conversations, `payload jsonb` ở submissions | thêm thông tin không cần migration; khi field ổn định thì mới tách cột |
| Lỗi | `internal/pkg/apperr` (Kind + Code + Message) | service trả 1 loại lỗi, HTTP/gRPC/WS tự map status — không rò stacktrace ra FE |
| Transaction nhiều repo | `TxManager` truyền `*gorm.DB` qua context | service không cần biết GORM |
| Phân trang | cursor `base64(created_at|id)` | timeline chat chèn liên tục, offset sẽ lệch |

### 5. Debug nhanh

- Log JSON có `request_id` (REST: header `X-Request-ID`, gRPC: metadata `x-request-id`).
- Bật SQL log: `POSTGRES_LOG_SQL=true`, cảnh báo query chậm: `POSTGRES_SLOW_QUERY=300ms`.
- `LOG_LEVEL=debug LOG_FORMAT=text` khi chạy local cho dễ đọc.
- Ghi log ra file để grep lại: `LOG_FILE=./logs/app.log` (append, `logs/` đã trong `.gitignore`).
  Xem: `make log-file`, tìm: `make log-grep Q=groups/refresh`.
- `LOG_STDOUT=false` (cần có `LOG_FILE`): console sạch hẳn, log chỉ vào file.
  Khi đó bật SQL bằng `make sql-on` thì `make dev-logs` **chỉ còn** câu SQL — xem rất dễ.
  `make sql-off` để tắt. **Chỉ dùng ở local**; prod phải `LOG_STDOUT=true` và `LOG_FILE` trống
  (file không tự rotate, docker/k8s thu gom log từ stdout).
- gRPC có reflection: `grpcurl -plaintext localhost:9090 list` (tắt ở prod bằng `GRPC_REFLECTION=false`).
- Health: `GET /healthz` (sống), `GET /readyz` (ping được DB).
- Ai đang online: `GET /api/v1/ws/online` (kết nối **trên instance này**) hoặc frame WS
  `presence.list` (đọc DB, đúng cả khi nhiều pod). Nếu DB ghi `online` mà API trả
  `offline` thì đúng — `last_seen_at` quá `PRESENCE_TTL`, xem `internal/domain/presence.go`.

---

## Bản đồ code: từ URL tới hàm

Nhìn một URL như `{{base_url}}/api/client/v1/chat/auth/me` mà muốn biết "code nằm ở đâu".

### 1. Tra nhanh bằng lệnh

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

### 2. Đọc URL để biết đang ở "cửa" nào

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

### 3. Luồng đi của một request

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

### 4. Tự tra bằng grep (khi không nhớ lệnh)

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

### 5. Chiều ngược lại: từ code ra URL

```bash
# Hàm service này được gọi từ những API nào?
grep -rn "msgs.Replies\|\.Replies(" internal/transport
```

### 6. Thứ không nằm trong `router.go`

| Việc | Ở đâu |
|---|---|
| Frame WebSocket (`message.send`, `typing`…) | `internal/transport/httpserver/ws/handler.go` — hàm `dispatch`, mỗi loại frame là một `case` |
| gRPC method | `api/proto/chat/v1/*.proto` (hợp đồng) → `internal/transport/grpcserver/*_handler.go` |
| Event Chat gửi CMS | `internal/cms/client.go` (`SendEvent`), gọi từ `internal/service/card` và `internal/service/outbox` |
| Job chạy nền | `internal/app/app.go` — mục `g.Go(...)` trong `Run()` (hub, đồng bộ CMS, outbox) |
| Wiring (ai phụ thuộc ai) | `internal/app/app.go` — đọc file này là thấy toàn bộ sơ đồ |

---

## Thêm / sửa / xoá field & API (checklist)

Nguyên tắc: **`migrations/` là nguồn sự thật của schema**, `internal/domain` là nguồn sự thật của
entity, `api/proto` là nguồn sự thật của API gRPC. Không bao giờ sửa file migration đã chạy;
luôn tạo file mới.

### A. Thêm 1 field (ví dụ: `messages.pinned`)

1. **Migration**
   ```bash
   make migrate-create name=add_pinned_to_messages
   ```
   ```sql
   -- up
   ALTER TABLE messages ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT false;
   CREATE INDEX IF NOT EXISTS ix_msg_pinned ON messages (conversation_id) WHERE pinned;
   -- down
   DROP INDEX IF EXISTS ix_msg_pinned;
   ALTER TABLE messages DROP COLUMN IF EXISTS pinned;
   ```
   ```bash
   make migrate-up
   ```

2. **Domain** — `internal/domain/message.go`
   ```go
   Pinned bool `gorm:"default:false;index" json:"pinned"`
   ```

3. **Repository** — thêm method nếu cần query mới (ví dụ `ListPinned`), khai báo ở
   `internal/repository/repository.go` rồi implement trong `internal/repository/postgres/message_repo.go`.

4. **Service** — thêm logic + kiểm tra quyền (`internal/service/message/service.go`).

5. **Transport**
   - REST: thêm field vào request struct trong `internal/transport/httpserver/handler/*.go`
     (response trả `domain.Message` nên field mới tự xuất hiện qua tag `json`).
   - gRPC: thêm field vào `api/proto/chat/v1/message.proto` (**số field mới, không tái dùng số cũ**),
     `make proto`, rồi map trong `internal/transport/grpcserver/mapper.go`.
   - WS: nếu là frame mới thì thêm hằng số + payload struct trong `ws/protocol.go` và 1 `case`
     trong `ws/handler.go`.

6. `make fmt test` và xong.

**Đường tắt:** nếu field chỉ để hiển thị / chưa chắc chắn, bỏ vào `metadata` (jsonb) của message
hoặc conversation — không cần migration, không cần sửa proto. Khi field đã ổn định và cần index
thì mới tách thành cột riêng.

### B. Sửa field

- Đổi **tên**: tạo migration `ALTER TABLE ... RENAME COLUMN`, sửa tag GORM trong domain. Với gRPC,
  **giữ nguyên số field**, chỉ đổi tên (số field mới là hợp đồng nhị phân, tên thì không).
- Đổi **kiểu**: làm 3 bước để không downtime — (1) thêm cột mới + backfill, (2) deploy code ghi/đọc
  cả hai, (3) migration xoá cột cũ.
- Đổi **ràng buộc** (`NOT NULL`, `CHECK`): nhớ backfill dữ liệu cũ trước khi thêm ràng buộc.

### C. Xoá field

1. Deploy code không còn dùng field (xoá khỏi service/transport, giữ cột trong DB).
2. Proto: **đánh dấu `reserved`**, đừng xoá hẳn số field:
   ```proto
   message Message { reserved 9; reserved "metadata_json"; }
   ```
3. Migration tiếp theo mới `DROP COLUMN`.

### D. Thêm 1 API mới

| Loại | Việc cần làm |
|---|---|
| REST | thêm handler trong `handler/`, đăng ký route trong `httpserver/router.go` |
| gRPC | thêm `rpc` vào `api/proto/chat/v1/service.proto` → `make proto` → thêm method vào `grpcserver/*_handler.go` |
| WS | thêm hằng `In...` + payload struct trong `ws/protocol.go` → thêm `case` trong `ws/handler.go` |

Nghiệp vụ **luôn** đặt ở `internal/service/...` để cả 3 transport dùng chung; handler chỉ parse và map.

### E. Thêm bảng mới

1. Migration `CREATE TABLE ...`
2. Struct trong `internal/domain/` + thêm vào `database.AllModels()` (để AutoMigrate ở local khớp).
3. Interface repository + implement GORM.
4. Service + transport theo nhu cầu.

### E2. Thêm 1 trạng thái task mới (ví dụ `waiting_qc`)

1. `internal/domain/task.go`: thêm hằng `AssignWaitingQC` + 1 dòng trong `AllowedAssignmentTransitions`
   (từ trạng thái nào sang được) và nhánh trong `DeriveTaskStatus` nếu ảnh hưởng trạng thái tổng.
2. Migration: nới `CHECK` constraint của `task_assignments.status` / `tasks.status`.
3. `internal/service/task/render.go`: thêm nhãn tiếng Việt.
4. `internal/service/task/lifecycle.go`: thêm 1 hàm ngắn gọi `applyTransition` (5-10 dòng).
5. Transport: thêm route REST + 1 RPC dùng lại `TaskActionRequest`.
6. `internal/domain/task_test.go`: thêm case vào bảng test.

### F. Hay quên

- `POSTGRES_AUTO_MIGRATE=true` chỉ dùng ở local; prod luôn `make migrate-up`.
- Mọi lỗi trả về FE phải là `apperr` (có `code` ổn định để FE bắt), không trả lỗi GORM thô.
- Query timeline phải dùng cursor (`paging.Page`), đừng dùng `OFFSET`.
- Sau khi sửa `.proto`: `make proto` rồi `make test` — mapper không khớp sẽ fail compile ngay.

### G. Migrate có mất dữ liệu không?

**Chiều lên (`make mu` / `make migrate-up`): không.** Toàn bộ `*.up.sql` trong repo chỉ có
`CREATE TABLE` / `ADD COLUMN` / `CREATE INDEX` / `INSERT` (seed bot). Ngoại lệ duy nhất là
`000005_fix_cms_unique_empty.up.sql`: nó `DROP INDEX` rồi tạo lại ngay (index không chứa dữ liệu),
và `UPDATE users SET open_id = NULL WHERE open_id = ''` — sửa giá trị rác, không xoá hàng.

Kiểm lại bất cứ lúc nào:

```bash
grep -nE "^\s*(DROP|TRUNCATE|DELETE)" migrations/*.up.sql
```

**Chiều xuống (`make mdown` / `make migrate-down`): CÓ, mất thật.**
`DROP COLUMN` của Postgres không undo được. Rollback rồi migrate lên lại thì cột quay về
nhưng **rỗng**.

| Lùi từ → đến | File chạy | Mất gì |
|---|---|---|
| 9 → 8 | `000009.down` | cột `conversation_members.pinned_at` (nhóm mỗi người đã ghim) |
| 8 → 7 | `000008.down` | cột `messages.mentions`, `messages.mention_all` (ai được nhắc @ trong mọi tin) |
| 7 → 6 | `000007.down` | chỉ xoá index — **không mất dữ liệu** |
| 6 → 5 | `000006.down` | cột `messages.pinned`, `pinned_at`, `pinned_by`, `users.presence_reason` |
| 5 → 4 | `000005.down` | chỉ đổi index — **không mất dữ liệu** |
| 4 → 3 | `000004.down` | **bảng** `notifications`, `webhook_outbox`, `idempotency_keys` + 13 cột CMS (`card`, `cms_profile`, `external_id`…) |
| 3 → 2 | `000003.down` | **bảng** `tasks`, `task_events`, `task_revisions`, `teams`, `team_members` |
| 2 → 1 | `000002.down` | bản ghi seed bot `@bot` |
| 1 → 0 | `000001.down` | **toàn bộ schema**: `messages`, `conversations`, `users`… |

Quy tắc:

- **Local**: `mdown` thoải mái, dữ liệu chỉ là seed (`make seed` dựng lại được).
- **Prod**: coi `mdown` như lệnh cấm. Sửa sai thì viết migration **mới** đi tiếp lên, đừng lùi.
  Bắt buộc lùi thì backup trước: `make prod-backup OUT=/backup/truoc-rollback.sql`.

**Hai lệnh mất nhiều hơn cả `mdown`**, dễ nhầm là của migrate:

```bash
make down-v        # docker compose down -v -> xoá luôn volume, mất sạch database
make dev-down-v    # tương tự cho stack dev
```

`make down` (không `-v`) thì an toàn — chỉ dừng container, volume còn nguyên.

---

## Phần còn lại của spec map vào kiến trúc

Spec Designer Bot còn nhiều tính năng ngoài phạm vi phase này. Dưới đây là chỗ đặt từng phần khi
làm tiếp, để không phải đổi kiến trúc.

| Tính năng trong spec | Đã có gì | Làm tiếp ở đâu |
|---|---|---|
| Chat nhóm theo bộ phận, avatar user | **xong** | — |
| Gửi chữ/ảnh/video/GIF xen kẽ | **xong** (`attachments` + `type`) | upload file là service riêng; chat chỉ lưu URL |
| Thread theo nhiệm vụ | **xong** (`messages.thread_id`) | — |
| Trạng thái đã xem / danh sách người đã đọc | **xong** (`last_read_at`, `message_receipts`) | — |
| Trạng thái OFF cạnh tên | **một phần** (`users.presence`) | job đồng bộ lịch OFF → `presence='off_duty'` |
| Tìm tin nhắn / tệp trong nhóm | **xong** cho tin nhắn | thêm endpoint `GET /conversations/:id/files` query bảng `attachments` |
| Tạo yêu cầu trong chat, gợi ý người phụ trách | **xong** (`internal/service/task`, gợi ý theo tổ + tải việc + OFF) | — |
| Nhận task / tạm dừng / treo / hoàn thành | **xong** (state machine ở `domain.AllowedAssignmentTransitions`) | — |
| Phiếu sửa | **xong** (`task_revisions`, hoàn thành riêng từng phiếu) | — |
| Gọi hỗ trợ khi OFF, phân công theo pool | **một phần**: `role=support` + gợi ý đã loại người OFF | worker tự gọi hỗ trợ khi phát hiện OFF |
| Gọi Edit, chia task nhiều người | **xong** (`role=edit`, mỗi người 1 card) | tách task con qua `tasks.parent_task_id` nếu cần |
| Chờ nhận, điều chỉnh phân công | **xong** (`?waiting_accept=true`, `/reassign`, `PATCH /tasks/:id`) | — |
| Role Admin/Order/Designer hiển thị khác nhau | `users.role` + `conversation_members.role` + `requireManager` cho task | tinh chỉnh thêm theo từng màn hình FE |
| Tính điểm theo định mức/số lượng | dữ liệu đã có ở `task_assignments.quota/quantity` khi `completed` | service tính điểm + đồng bộ Drive |
| Đơn hàng, Drive, tính điểm, Drive Assets | — | service ngoài, gọi vào chat qua **gRPC** (`ChatService.SendMessage`) để post tin nhắn hệ thống |
| Tick dùng AI | — | bot riêng: `botSvc.Register("ai", ...)` — xem [Bot @bot và mẫu (template)](backend.md#bot-bot-và-mẫu-template) §5 |

Hai mở rộng kỹ thuật nên tính trước:

- **Outbox cho event**: nếu sau này không được mất event realtime, thêm bảng `outbox_events` ghi
  trong cùng transaction với message, rồi worker publish (thay cho `hub.Publish` trực tiếp).
- **Partition bảng `messages`** theo tháng khi dữ liệu lớn; index hiện tại
  (`conversation_id, created_at DESC, id DESC`) đã sẵn cho kiểu truy vấn này.
