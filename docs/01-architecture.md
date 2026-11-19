# 01 — Kiến trúc

## 1. Tầng và trách nhiệm

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

## 2. Luồng gửi 1 tin nhắn (quan trọng nhất)

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

## 3. Realtime và scale nhiều instance

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

## 4. Những quyết định thiết kế đáng chú ý

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

## 5. Debug nhanh

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
