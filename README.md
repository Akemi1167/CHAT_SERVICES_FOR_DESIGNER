# chat-service

Service chat cho ABC Workspace: **Go + Fiber (REST/WebSocket) + gRPC + PostgreSQL + GORM**.

Đã có:

- Chat riêng (1-1) và chat nhóm, có phân quyền owner/admin/member
- WebSocket: FE gửi `ws://host/ws?token=<JWT>`, server xác thực token rồi đẩy tin nhắn realtime
- Tin nhắn text / ảnh / video / GIF / file (attachment), reply, **thread theo nhiệm vụ**
- Trạng thái đã xem (unread count + danh sách người đã đọc), typing, presence online/OFF
- Tìm kiếm tin nhắn theo nội dung / mã task / người gửi / khoảng ngày
- **Bot `@bot` + gửi mẫu**: parse `@bot <mã_mẫu> field=value`, validate theo schema của mẫu rồi **lưu xuống DB** (`bot_submissions`) và trả lời trong chat
- gRPC `ChatService` / `BotService` cho service-to-service
- **Task**: tạo yêu cầu trong chat (tự gợi ý người phụ trách theo tổ + tải việc), **nhận task** (chọn định mức & số lượng), tạm dừng / tiếp tục / treo / hoàn thành, chia task cho nhiều người (mỗi người 1 card), gọi hỗ trợ / gọi Edit, đổi người phụ trách, danh sách **chờ nhận**, lịch sử task
- **Reply & thread**: `reply_to_id` (trích dẫn) tách khỏi `thread_id` (thread của yêu cầu), kèm `reply_count` / `thread_count` trong mọi danh sách và API `GET /messages/:id/replies?mode=direct|thread`
- **Phiếu sửa**: ghi nguyên nhân / loại sửa / số lượng, tự mở lại card đã hoàn thành, hoàn thành riêng từng phiếu
- **Tích hợp CMS/Task Engine**: xác thực token CMS bằng JWKS (RS256), đồng bộ nhóm + thành viên từ CMS, lưu card `abc.card/v1` và lọc nút theo `visibleTo`, webhook ký HMAC (`form.submit`, `card.action`, `message.created`), Bot API cho CMS gọi sang (có `Idempotency-Key`), API `/api/client/v1/chat/*` theo hợp đồng — xem [docs/10-cms-integration.md](docs/10-cms-integration.md)
- Idempotency khi gửi tin (chống trùng khi FE retry), cursor pagination, graceful shutdown

---

## 1. Chạy nhanh

**Cách 1 — chạy source trong Docker, hot reload (khuyên dùng, không cần cài Go):**

```bash
make dev-up      # postgres + redis + migrate + app (air tự build lại khi sửa code)
make dev-logs
```

**Cách 2 — chạy bản build giống prod trong Docker:**

```bash
make up          # postgres + redis + migrate + app (image alpine ~60MB)
make logs
```

**Cách 3 — chạy bằng Go ở máy, chỉ DB trong Docker:**

```bash
cp .env.example .env            # sửa JWT_SECRET nếu cần
make up-deps                    # postgres + redis
make migrate-up                 # tạo schema + seed bot "@bot" + seed tổ
make run                        # HTTP :8080, gRPC :9090
```

Chi tiết Docker: [docs/08-docker.md](docs/08-docker.md).

Sinh token để test (giống token FE sẽ gửi xuống):

```bash
TOKEN=$(make -s token SUB=user-001 NAME="Nguyen Van A" ROLE=designer)
curl -s localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN" | jq
```

Thử luồng chat + bot + task:

```bash
# 1) tạo nhóm
curl -s -X POST localhost:8080/api/v1/conversations \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"group","name":"To Design"}' | jq

CONV=<conversation_id ở trên>

# 2) gửi mẫu cho bot ngay trong tin nhắn
curl -s -X POST localhost:8080/api/v1/conversations/$CONV/messages \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"@bot yeu_cau tieu_de=\"Banner Tet\" to=design so_luong=3"}' | jq

# 3) tạo yêu cầu bằng bot -> task tự sinh mã T-000001, tự gợi ý người phụ trách
curl -s -X POST localhost:8080/api/v1/conversations/$CONV/messages \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"@bot yeu_cau tieu_de=\"Banner Tet\" to=design so_luong=3 dinh_muc=A4"}' | jq

# 4) xem task + nhận task (chọn định mức + số lượng)
curl -s "localhost:8080/api/v1/tasks?conversation_id=$CONV" -H "Authorization: Bearer $TOKEN" | jq
curl -s -X POST localhost:8080/api/v1/tasks/<task_id>/accept \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"quota":"A4","quantity":3}' | jq

# 5) phiếu sửa
curl -s -X POST localhost:8080/api/v1/tasks/<task_id>/revisions \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"reason":"Sai mau nen","revision_type":"mau","quantity":2}' | jq
```

> Tổ design chưa có thành viên thì bot không gợi ý được người phụ trách. Thêm bằng:
> `POST /api/v1/teams/<team_id>/members` với `{"user_ids":["..."]}` (lấy `team_id` từ `GET /api/v1/teams`).

Test WebSocket bằng `websocat`:

```bash
websocat "ws://localhost:8080/ws?token=$TOKEN"
# gõ:
{"id":"1","type":"message.send","payload":{"conversation_id":"<CONV>","content":"hello"}}
{"id":"2","type":"template.send","payload":{"conversation_id":"<CONV>","bot_slug":"bot","template_code":"phieu_sua","payload":{"ma_task":"T-100","nguyen_nhan":"sai mau","loai_sua":"mau","so_luong":1}}}
```

---

## 1b. Hai chế độ chạy

| | **Fake (chưa cần CMS)** — mặc định | **Tích hợp CMS** |
|---|---|---|
| Bật bằng | giữ `CMS_ENABLED=false` | `CMS_ENABLED=true` + `CHAT_SERVICE_KEY` |
| Token | `make token SUB=an NAME=An ROLE=order` (HS256) | access token do CMS phát (RS256, verify bằng JWKS) |
| Nhóm & thành viên | tự tạo bằng `POST /api/v1/conversations` | CMS là nguồn sự thật (poll + `PUT /bot/v1/groups/:id`) |
| Task / card | `@bot yeu_cau …` tạo task nội bộ khi `TASK_ENGINE_ENABLED=true` | CMS tạo đơn và dựng card, Chat chỉ lưu + lọc `visibleTo` |
| Tệp trong nhóm | lưu `attachments` trong tin nhắn (API liệt kê tệp do BE/CMS cung cấp) | như trái |

FE làm giao diện trước thì dùng chế độ fake: chat, reply, thread, đã đọc, tìm kiếm, realtime đều chạy
đầy đủ, không phụ thuộc CMS. Khi CMS sẵn sàng chỉ cần đổi env và đổi nguồn token, **không phải sửa
code FE** vì đường dẫn + payload giữ nguyên.

## 2. Cấu trúc thư mục

```
.
├── api/proto/chat/v1/        # hợp đồng gRPC (.proto) — nguồn sự thật của API nội bộ
├── gen/go/                   # code sinh từ proto (make proto), KHÔNG sửa tay
├── cmd/
│   ├── server/               # entrypoint service
│   └── token/                # tiện ích sinh JWT để test
├── migrations/               # SQL migration (golang-migrate) — nguồn sự thật của schema
├── internal/
│   ├── app/                  # WIRING: tạo mọi dependency, start/stop (đọc file này hiểu cả hệ thống)
│   ├── config/               # đọc env -> struct Config
│   ├── logger/               # slog (log JSON, có request_id)
│   ├── database/             # kết nối Postgres/GORM + danh sách model
│   ├── domain/               # ENTITY + enum + JSONMap (tầng trong cùng, không import gì của app)
│   ├── repository/           # interface truy cập dữ liệu (port)
│   │   └── postgres/         # implement bằng GORM (+ transaction qua context)
│   ├── service/              # BUSINESS LOGIC
│   │   ├── auth/             # verify JWT, upsert user, presence
│   │   ├── conversation/     # chat riêng/nhóm, thành viên, quyền
│   │   ├── message/          # gửi/đọc tin, thread, đã xem, điều phối bot, tin nhắn hệ thống
│   │   ├── bot/              # parse "@bot ...", registry handler, lưu mẫu
│   │   └── task/             # task, nhận task, phiếu sửa, tổ + gợi ý người phụ trách
│   ├── realtime/             # Hub + Client + Broker (memory | redis) để fanout event
│   ├── transport/            # ADAPTER vào ra
│   │   ├── grpcserver/       # gRPC handler + interceptor + mapper domain<->proto
│   │   └── httpserver/       # Fiber: router, middleware, handler REST, ws/
│   └── pkg/                  # tiện ích dùng chung (apperr, jwtauth, paging, httpresp)
└── docs/                     # tài liệu kiến trúc, giao thức WS, bot, cách đổi schema
```

Nguyên tắc phụ thuộc (chỉ chảy một chiều, dễ test & dễ sửa):

```
transport (http/grpc/ws)  ->  service  ->  repository (interface)  ->  postgres/gorm
                                   |
                                   +-> domain (entity, không phụ thuộc ai)
```

---

## 3. Tài liệu

| File | Nội dung |
|------|----------|
| [docs/01-architecture.md](docs/01-architecture.md) | Sơ đồ tầng, luồng gửi tin nhắn, vì sao chia như vậy, cách scale nhiều instance |
| [docs/02-websocket.md](docs/02-websocket.md) | Giao thức WebSocket đầy đủ: cách FE auth, từng frame, ví dụ client JS |
| [docs/03-bot-template.md](docs/03-bot-template.md) | `@bot` + gửi mẫu: cú pháp, schema mẫu, bảng DB, cách thêm bot/mẫu mới |
| [docs/04-them-sua-xoa-field.md](docs/04-them-sua-xoa-field.md) | Quy trình chuẩn khi thêm/sửa/xoá field hoặc API (checklist từng bước) |
| [docs/05-api.md](docs/05-api.md) | Danh sách REST endpoint + gRPC RPC |
| [docs/06-roadmap.md](docs/06-roadmap.md) | Phần còn lại của spec (Drive, tính điểm, AI...) map vào kiến trúc hiện tại |
| [docs/07-task.md](docs/07-task.md) | Task: mô hình dữ liệu, state machine, tạo yêu cầu / nhận task / phiếu sửa, chia task |
| [docs/08-docker.md](docs/08-docker.md) | Hai stack Docker (dev hot-reload và prod-like), lệnh, biến môi trường, sự cố hay gặp |
| [docs/09-postman.md](docs/09-postman.md) | Postman: file import, cách chạy cả collection, WebSocket & gRPC |
| [docs/14-code-map.md](docs/14-code-map.md) | **Từ URL tới code**: route nằm đâu, handler nào, service nào — kèm `make route` |
| [docs/13-test-guide.md](docs/13-test-guide.md) | **Cách test**: `make seed`, chat trên terminal, Postman, curl, xem log khi lỗi |
| [docs/12-deploy.md](docs/12-deploy.md) | **Deploy VPS**: stack prod, TLS/WebSocket qua nginx, bảo mật, backup, checklist |
| [docs/17-giao-dien-live.md](docs/17-giao-dien-live.md) | **Giao diện live** `/demo/live.html` — bản chạy thật của `chat-thread-demo.html` |
| [docs/16-api-flow.md](docs/16-api-flow.md) | **Luồng gọi API theo thứ tự** (từ FE): mở app, mở nhóm, gửi tin, thread, tạo yêu cầu, bấm nút card |
| [docs/11-frontend.md](docs/11-frontend.md) | **Dành cho FE**: 2 chế độ, auth, REST + WebSocket, reply/thread, card, luồng chuẩn, checklist |
| [docs/15-cms-cutover.md](docs/15-cms-cutover.md) | **Nối vào CMS**: xin gì, đưa gì, điền env, `make cms-check`, cách quay lui |
| [docs/10-cms-integration.md](docs/10-cms-integration.md) | **Tích hợp CMS**: JWKS, đồng bộ nhóm, card/visibleTo, webhook HMAC, Bot API, API cho FE |

---

## 4. Lệnh hay dùng

```bash
make help            # xem tất cả lệnh
make dev-up / dev-logs / dev-test / dev-sh / dev-down     # stack dev trong docker (hot reload)
make proto           # sinh lại code gRPC sau khi sửa .proto
make migrate-create name=add_pinned_to_messages
make migrate-up / migrate-down / migrate-status
make mu / ms / mn name=... / mdown / mforce version=...   # phím tắt của 5 lệnh migrate
make test / lint / fmt
make up / down / logs / psql
make redis-cli       # redis-cli, tự dùng REDIS_PASSWORD trong .env
make sql-on / sql-off   # bật/tắt in câu SQL ra console docker
make log-file / log-grep Q=...   # xem log đã ghi ra ./logs/app.log
make seed            # dựng dữ liệu demo + in token/ID để test ngay
make token SUB=user-002 ROLE=order
make postman         # sinh lại collection Postman sau khi đổi API
make postman-run TOKEN=$(make -s token)   # chạy cả collection bằng Postman CLI
make postman-flow TOKEN=$(make -s token) [STOP=2.3]   # chạy folder FLOW, dừng ở bước muốn xem
```

## 4b. Postman

Import 2 file này vào Postman (Import → File):

- `internal/postman/chat-service.postman_collection.json` — 79 request, 10 folder, có sẵn test + tự lưu id giữa các request.
  Folder **`FLOW`** là 16 bước đánh số theo đúng thứ tự thật — chạy cái này trước nếu mới làm quen.
  Folder **`10 · WebSocket & gRPC`** chứa mẫu frame WS (body + các trường hợp xảy ra) trong tab Documentation.
- `internal/postman/chat-service.local.postman_environment.json` — environment `Chat Service · Local`

Rồi dán token (`make token`) vào biến `token`. Chi tiết + cách thêm request WebSocket/gRPC:
[docs/09-postman.md](docs/09-postman.md).

## 5. Biến môi trường

Xem [.env.example](.env.example) — mọi config đều qua env, không hardcode. Hai biến bắt buộc:
`POSTGRES_DSN` và `JWT_SECRET` (service sẽ không khởi động nếu thiếu).

## 6. Yêu cầu hệ thống

- Go 1.26+ (các dependency gRPC/x-net yêu cầu mức này)
- PostgreSQL 14+ (dùng `pgcrypto`, `pg_trgm`, index partial)
- Redis 7+ (chỉ cần khi chạy >1 instance, bật `REDIS_ENABLED=true`)
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` (chỉ cần khi sửa proto — `make setup`)
