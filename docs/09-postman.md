# 09 — Postman

Thư mục `postman/` có 2 dạng, cùng nội dung REST nhưng dùng cho 2 việc khác nhau:

| File / thư mục | Dùng để | Có WebSocket & gRPC? |
|---|---|---|
| `postman/chat-service.postman_collection.json` | **Import vào app Postman** (Import → File) | Không — Postman không export được 2 protocol này ra v2.1 |
| `postman/chat-service.local.postman_environment.json` | Import environment đi kèm | — |
| `postman/collections/chat-service/` | Chạy bằng **Postman CLI** / review trong Git (định dạng v3) | Có (folder `08 · WebSocket`, `09 · gRPC`) |
| `postman/environments/Chat Service Local.environment.yaml` | Environment cho CLI | — |

Cả hai được sinh từ `scripts/gen_postman.py` (`make postman`): sửa API thì sửa script rồi chạy lại,
đừng sửa tay file JSON. Script cũng tự dịch sang bản v3 và **giữ nguyên** 2 folder WebSocket/gRPC
(viết tay, vì v2.1 không chứa được).

## 1. Import và dùng (3 bước)

1. Chạy service: `make dev-up` (hoặc `make up-deps && make migrate-up && make run`).
2. Postman → **Import** → chọn 2 file:
   `postman/chat-service.postman_collection.json` và `postman/chat-service.local.postman_environment.json`.
3. Chọn environment **Chat Service · Local**, lấy token dev rồi dán vào biến `token`:

```bash
make token SUB=user-001 NAME="Nguyen Van A" ROLE=designer
```

Chạy lần lượt folder `00` → `06`. Mỗi request tự lưu id cần thiết vào collection variable
(`user_id`, `conversation_id`, `message_id`, `thread_id`, `team_id`, `task_id`, `task_code`,
`assignment_id`, `revision_id`) nên folder sau dùng được ngay, không phải copy id bằng tay.

Muốn thử phân công 2 người (chia task, đổi người phụ trách, chat riêng): tạo token thứ 2
(`make token SUB=user-002 ROLE=order`), gọi `GET /api/v1/me` bằng token đó, rồi dán `user_id`
nhận được vào biến `user_id_2`.

## 2. Nội dung collection (56 request / 9 folder)

| Folder | Nội dung |
|---|---|
| `00 · Bắt đầu` | healthz, readyz, `/me` (lưu `user_id`) |
| `01 · Hội thoại` | tạo nhóm / chat riêng, danh sách, chi tiết, thêm thành viên |
| `02 · Tin nhắn` | gửi text / ảnh / trong thread, lịch sử, tìm kiếm, đã đọc, ai đã xem, xoá |
| `03 · Tổ & gợi ý` | danh sách tổ, thêm thành viên, **gợi ý người phụ trách** |
| `04 · Bot @bot & gửi mẫu` | schema mẫu, `@bot` help, `@bot yeu_cau`, submit mẫu từ form, mẫu đã lưu |
| `05 · Task` | tạo, danh sách, **chờ nhận**, nhận (định mức+số lượng), tạm dừng/tiếp tục/treo/hoàn thành, chia task, đổi người, điều chỉnh, lịch sử |
| `06 · Phiếu sửa` | tạo phiếu, danh sách, hoàn thành từng phiếu, hoàn thành card |
| `07 · Lỗi & quyền` | 401 thiếu token, 400 UUID sai, 403 không phải thành viên, 404 route sai, 400 tin nhắn rỗng |
| `99 · Dọn dẹp` | huỷ task, rời nhóm, bỏ khỏi tổ (chạy sau cùng) |
| `08 · CMS → Chat (Bot API)` | CMS gọi sang Chat: resolve user, gửi/sửa card, thread, thông báo, đẩy nhóm |
| `09 · API cho FE (hợp đồng CMS)` | `/api/client/v1/chat/*`: bootstrap, tin nhắn, bấm nút card, gửi form, thông báo |

Mỗi folder cần cấu hình khác nhau:

| Folder | Cần gì |
|---|---|
| `00`–`04`, `07`, `99` | chạy với cấu hình mặc định |
| `05`, `06` (task nội bộ) | `TASK_ENGINE_ENABLED=true` (stack dev đã bật sẵn) |
| `08`, `09` (CMS) | `CMS_ENABLED=true` + biến `service_key`, `cms_token`, `chat_id` — xem [10-cms-integration.md](10-cms-integration.md) |

```bash
make postman-run TOKEN=$(make -s token)                 # tất cả trừ folder CMS
make postman-run-cms TOKEN=<access token CMS> KEY=<CHAT_SERVICE_KEY> CHAT_ID=wg_...
```

Mỗi request có mô tả (ràng buộc, mã lỗi có thể gặp) và test script kiểm tra status + trường quan trọng.

## 3. Chạy tự động (CI)

```bash
make postman-run TOKEN=$(make -s token) USER2=<uuid-nguoi-thu-2>
```

Kết quả lần chạy mới nhất trên stack dev: **56 request, 82 assertion, 0 fail**.

Bản v3 chạy được cả WebSocket/gRPC nhưng Postman CLI **yêu cầu đăng nhập** khi collection có
request gRPC (`postman login`), nên lệnh trên dùng file v2.1 (REST) để chạy offline.
Lint bản v3: `make postman-lint`.

## 4. WebSocket & gRPC trong app Postman

Postman không export 2 loại này ra file v2.1
([tài liệu Postman](https://learning.postman.com/docs/getting-started/importing-and-exporting/exporting-data)),
nên sau khi import bạn tự tạo (mỗi cái 1 phút):

**WebSocket**: New → WebSocket → URL `ws://localhost:8080/ws?token=<JWT>` → Connect.
Dán frame để gửi, ví dụ:

```json
{"id":"1","type":"message.send","payload":{"conversation_id":"<id>","content":"hello","client_msg_id":"c1"}}
```

Các frame khác (`template.send`, `message.read`, `typing`, `message.delete`, `ping`) đã viết sẵn trong
`postman/collections/chat-service/08 · WebSocket/.resources/ws-chat.resources/messages/` — copy nội dung
`content` sang Postman là dùng được. Giao thức đầy đủ: [02-websocket.md](02-websocket.md).

**gRPC**: New → gRPC → URL `localhost:9090` → Postman tự lấy service qua **server reflection**
(`GRPC_REFLECTION=true`) → chọn method → tab **Metadata** thêm `authorization: Bearer <JWT>`.
Message mẫu cho 9 RPC hay dùng nằm trong `postman/collections/chat-service/09 · gRPC/*.request.yaml`.
Nếu prod tắt reflection thì nạp proto từ `api/proto/chat/v1/`.

## 5. Khi API đổi

1. Sửa `scripts/gen_postman.py` (thêm/sửa request, mô tả, assertion).
2. `make postman` → sinh lại cả 2 dạng.
3. `make postman-lint` và `make postman-run TOKEN=...` để chắc chắn còn chạy đúng.
