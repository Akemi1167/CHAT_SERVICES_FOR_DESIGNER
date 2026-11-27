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
- **Tích hợp CMS/Task Engine**: xác thực token CMS bằng JWKS (RS256), đồng bộ nhóm + thành viên từ CMS, lưu card `abc.card/v1` và lọc nút theo `visibleTo`, webhook ký HMAC (`form.submit`, `card.action`, `message.created`), Bot API cho CMS gọi sang (có `Idempotency-Key`), API `/api/client/v1/chat/*` theo hợp đồng — xem [docs/backend.md](docs/backend.md#tích-hợp-cms-designer-bot)
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

Chi tiết Docker: [docs/deploy.md](docs/deploy.md#docker).

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

# 3) gửi mẫu cho bot (Chat chỉ lưu mẫu; đơn/task do CMS sở hữu)
curl -s -X POST localhost:8080/api/v1/conversations/$CONV/messages \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"content":"@bot yeu_cau tieu_de=\"Banner Tet\" to=design so_luong=3 dinh_muc=A4"}' | jq
```


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
| Task / card | `@bot yeu_cau …` chỉ lưu mẫu (không có module task nội bộ) | CMS tạo đơn và dựng card, Chat chỉ lưu + lọc `visibleTo` |
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
| [docs/FLOW.md](docs/FLOW.md) | **Đọc trước tiên — cho cả 3 đội**: sơ đồ FE ↔ Chat ↔ CMS · ai sở hữu cái gì · 4 danh mục `type` (6 frame gửi lên, 12 frame nhận về, 4 event webhook, `action` trên card) · khi nào CMS cần gọi Chat · 9 luồng A–I kèm **lệnh chạy thử dán là chạy** · bảng tra lỗi |
| [docs/source.md](docs/source.md) | **Sửa code**: sơ đồ tầng, luồng gửi tin nhắn, cách scale nhiều instance · từ URL tới hàm (`make route`) · checklist thêm/sửa/xoá field & API · migrate có mất dữ liệu không · phần còn lại của spec map vào đâu |
| [docs/backend.md](docs/backend.md) | **API & tích hợp**: danh sách REST + gRPC · tích hợp CMS (JWKS, đồng bộ nhóm, card/`visibleTo`, webhook HMAC, Bot API) · `@bot` và mẫu · module task nội bộ · Postman · hướng dẫn test |
| [docs/frontend.md](docs/frontend.md) | **Dành cho FE**: hai bề mặt API khác nhau ở đâu · giao thức WebSocket đầy đủ · thứ tự gọi API theo từng tình huống · giao diện mẫu `/demo/live.html` chạy thật |
| [docs/deploy.md](docs/deploy.md) | **Đưa lên server**: hai stack Docker · deploy VPS (TLS/WebSocket qua nginx, bảo mật, backup) · cutover bật tích hợp CMS |
| [docs/cms-goi-chat.md](docs/cms-goi-chat.md) | **Dành cho đội CMS**: khi CMS có thay đổi (tạo đơn, đổi trạng thái, sửa card, đổi thành viên) thì gọi API nào của Chat — body, response, `Idempotency-Key`, `cardVersion`, mã lỗi, ví dụ trọn vòng |
| [docs/CHAT_SERVICE_INTEGRATION.md](docs/CHAT_SERVICE_INTEGRATION.md) | **Spec gốc do CMS đưa** — giữ nguyên, không sửa. Hợp đồng hai phía: token, webhook, Bot API, Client API |

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
make postman-flow TOKEN=$(make -s token) [STOP=2.3]   # bản đầy đủ: FLOW 17 bước
make postman-fe   TOKEN=$(make -s token-cms CMS_USER=6)   # bản [FE]
make postman-cms  KEY=$CHAT_SERVICE_KEY                   # bản [CMS]
```

## 4b. Postman

Import 2 file này vào Postman (Import → File):

Có **3 bản** — gửi cho ai thì import bản của người đó:

| File | Gửi cho | Nội dung |
|---|---|---|
| `chat-service-fe.postman_collection.json` | **đội FE** | `FLOW · Luồng của FE` (12 bước) + `/api/client/v1/chat/*` + mẫu WebSocket |
| `chat-service-cms.postman_collection.json` | **đội CMS** | `FLOW · Luồng của CMS` (12 bước) + `/bot/v1/*` |
| `chat-service.postman_collection.json` | nội bộ | bản đầy đủ: 83 request, 10 folder, cả `/api/v1` và lỗi/quyền |
⚠️ Khi `CMS_ENABLED=true`, bước gửi form **tạo đơn thật** bên CMS.
  Folder **`10 · WebSocket & gRPC`** chứa mẫu frame WS (body + các trường hợp xảy ra) trong tab Documentation.
- `internal/postman/chat-service.local.postman_environment.json` — environment `Chat Service · Local`

Rồi dán token (`make token`) vào biến `token`. Chi tiết + cách thêm request WebSocket/gRPC:
[docs/backend.md](docs/backend.md#postman).

## 5. Biến môi trường

Xem [.env.example](.env.example) — mọi config đều qua env, không hardcode. Hai biến bắt buộc:
`POSTGRES_DSN` và `JWT_SECRET` (service sẽ không khởi động nếu thiếu).

## 6. Yêu cầu hệ thống

- Go 1.26+ (các dependency gRPC/x-net yêu cầu mức này)
- PostgreSQL 14+ (dùng `pgcrypto`, `pg_trgm`, index partial)
- Redis 7+ (chỉ cần khi chạy >1 instance, bật `REDIS_ENABLED=true`)
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` (chỉ cần khi sửa proto — `make setup`)
