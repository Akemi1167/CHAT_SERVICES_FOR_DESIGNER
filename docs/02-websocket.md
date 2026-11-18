# 02 — Giao thức WebSocket

## 1. Kết nối & xác thực

```
GET ws://HOST/ws?token=<JWT>
```

Server nhận token theo thứ tự **`?token=` → `Sec-WebSocket-Protocol` → header `Authorization`**
(REST thì ngược lại: header trước). Lý do ưu tiên query ở WebSocket: browser không set được header
khi mở WebSocket, và các tool hay tự gắn header `Authorization` kế thừa (ví dụ request WebSocket
lưu trong Postman collection có Bearer auth) — nếu header thắng thì tab sẽ đăng nhập bằng **người
khác**, vẫn nhận `ready` nhưng không bao giờ nhận tin của room.

Frame `ready` trả về `user_id`, `external_id`, `display_name`, `role` — luôn kiểm tra frame này để
biết tab đang là ai.

```js
const ws = new WebSocket(`wss://host/ws?token=${jwt}`);
```

Token sai/hết hạn → server trả frame `error` rồi đóng kết nối (close code chuẩn).
Token hợp lệ → user được upsert vào bảng `users` (theo claim `sub`), presence = `online`,
và server gửi ngay frame `ready`.

Claim được dùng: `sub` (bắt buộc, = external user id), `name`, `email`, `avatar`, `role`, `tenant_id`.

## 2. Dạng frame

Mọi frame là một JSON object.

Client → Server:
```json
{ "id": "<id request do FE tự sinh>", "type": "<loại>", "payload": { } }
```

Server → Client:
```json
{ "type": "<loại>", "ref": "<id request nếu là ack/error>", "payload": { }, "ts": "..." }
```

## 3. Frame client gửi lên

### `message.send` — gửi tin nhắn (text/ảnh/video/file, reply, thread, và cả `@bot`)

> Phân biệt 2 quan hệ: `reply_to_id` = tin được **trích dẫn** (nút Reply), `thread_id` = tin **mở
> thread** (card/yêu cầu). Một tin có thể có cả hai. Lấy phản hồi:
> `GET /api/v1/messages/:id/replies?mode=direct|thread`.
```json
{"id":"c1","type":"message.send","payload":{
  "conversation_id":"uuid",
  "type":"text",                       // text|image|video|file|gif (mặc định text)
  "content":"nội dung hoặc @bot yeu_cau so_luong=3",
  "thread_id":"uuid|null",             // trả lời trong thread của 1 nhiệm vụ
  "reply_to_id":"uuid|null",
  "metadata":{"task_code":"T-100"},    // tuỳ ý, lưu jsonb
  "attachments":[{"kind":"image","url":"https://...","name":"a.png","mime_type":"image/png","size_bytes":1024,"width":800,"height":600}],
  "client_msg_id":"uuid-do-FE-sinh"    // nên luôn gửi: chống trùng khi retry
}}
```

> **`client_msg_id` phải DUY NHẤT cho mỗi tin nhắn.** Gửi lại đúng giá trị cũ bị coi là retry:
> server trả `ack` với `"duplicated": true`, **không tạo tin mới và không broadcast** — người gửi
> tưởng đã gửi, còn người khác không nhận được gì. FE phải sinh UUID mới cho mỗi tin
> (`crypto.randomUUID()`), chỉ giữ nguyên khi *gửi lại chính tin đó* sau khi mất kết nối.
> Khi test tay (Postman) thì cách an toàn nhất là **bỏ hẳn trường này** — server sẽ luôn tạo tin mới.
> Server có log: `bo qua tin nhan trung client_msg_id (khong broadcast)`.

### `template.send` — gửi mẫu cho bot từ form (không cần gõ `@bot`)
```json
{"id":"c2","type":"template.send","payload":{
  "conversation_id":"uuid",
  "thread_id":"uuid|null",
  "bot_slug":"bot",
  "template_code":"phieu_sua",
  "payload":{"ma_task":"T-100","nguyen_nhan":"sai màu","loai_sua":"màu","so_luong":1},
  "client_msg_id":"uuid"
}}
```

### `message.read` — đánh dấu đã đọc
```json
{"id":"c3","type":"message.read","payload":{"conversation_id":"uuid","message_id":"uuid"}}
```

### `message.delete` — xoá mềm (người gửi hoặc admin/owner nhóm)
```json
{"id":"c4","type":"message.delete","payload":{"message_id":"uuid"}}
```

### `typing` — đang nhập (không lưu DB, không ack)
```json
{"id":"c5","type":"typing","payload":{"conversation_id":"uuid","typing":true}}
```

### `ping` — giữ kết nối / đo RTT (server cũng tự ping ở tầng protocol mỗi `WS_PING_INTERVAL`)
```json
{"id":"c6","type":"ping"}
```

## 4. Frame server gửi xuống

| type | khi nào | payload |
|---|---|---|
| `ready` | ngay sau khi auth xong | `{user_id, external_id, display_name, role, conn_id, server_now}` — dùng để xác nhận tab đang đăng nhập bằng ai |
| `ack` | request của chính client thành công | tuỳ loại; với `message.send`: `{client_msg_id, message, bot_reply, submission, duplicated}` |
| `error` | request lỗi | `{code, message, details}` + `ref` = id request |
| `pong` | trả `ping` | `{server_now}` |
| `message.new` | có tin nhắn mới trong hội thoại mình là thành viên (kể cả tin của bot) | object message đầy đủ |
| `message.deleted` | tin nhắn bị xoá | `{conversation_id, message_id}` |
| `message.read` | có người đọc tin | `{conversation_id, message_id, user_id, seen_at}` |
| `typing` | người khác đang nhập (không gửi lại cho chính người đó) | `{conversation_id, user_id, typing}` |
| `presence.update` | user online/offline | `{user_id, online}` |
| `conversation.created` / `conversation.updated` | được thêm vào nhóm, nhóm đổi thành viên | object conversation |
| `bot.submission.stored` | bot đã lưu 1 mẫu | object submission |
| `task.created` | có yêu cầu/task mới trong hội thoại | `{task}` |
| `task.updated` | task đổi trạng thái, đổi phân công, có phiếu sửa... | `{task, event_type, ...}` — vẽ lại card từ `task` |

Mọi diễn biến task cũng đến dưới dạng `message.new` (tin nhắn hệ thống, `metadata.kind` =
`task_card` \| `task_event`) để hiển thị trong dòng chat.

Mã lỗi hay gặp: `unauthenticated`, `not_conversation_member`, `conversation_not_found`,
`invalid_uuid`, `empty_message`, `bot_template_not_found`, `bot_template_missing_fields`.

## 5. Cách test nhanh (nhiều người cùng lúc)

**Cách 1 — terminal (nhanh nhất, không cần Postman).** Mỗi cửa sổ là một người:

```bash
# cửa sổ 1
make ws SUB=an   NAME=An   CONV=<conversation_id>
# cửa sổ 2
make ws SUB=binh NAME=Binh CONV=<conversation_id>
```

Gõ text rồi Enter là gửi; cửa sổ kia thấy ngay. Lệnh trong client: `/ping`, `/typing`,
`/read <message_id>`, `/json {...}` (gửi frame tuỳ ý), `/quit`.
Client tự ký token bằng `JWT_SECRET` trong `.env` nên không cần `make token`;
muốn dùng token có sẵn thì `go run ./cmd/wsclient -token <JWT> -conv <id>`.

Lấy `conversation_id`: tạo nhóm bằng `POST /api/v1/conversations`, hoặc
`GET /api/v1/conversations` rồi copy `data[0].id`. Lưu ý mỗi `SUB` là một user khác nhau,
và user phải **là thành viên** của hội thoại đó mới gửi/nhận được.

**Cách 2 — Postman.** New → WebSocket → dán `ws://localhost:8080/ws?token=<JWT>` → **Connect**.
Mỗi người mở một tab riêng (Ctrl/Cmd+N), các tab giữ kết nối song song.
Token phải là JWT thật (chạy `make token SUB=an NAME=An ROLE=order`), không phải chữ `<TOKEN_AN>`.

Thứ tự quan trọng: **Connect hết các tab trước, rồi mới gửi** — WebSocket không phát lại tin cũ,
ai kết nối sau sẽ không thấy tin đã gửi trước đó (muốn xem thì gọi
`GET /api/v1/conversations/:id/messages`).

Ba cách tự kiểm tra khi "không thấy tin nhắn":

1. Nhìn `ack`: nếu có `"duplicated": true` thì tin **không được gửi đi** vì trùng
   `client_msg_id` — đổi giá trị khác hoặc bỏ trường đó đi.
2. Trong từng tab, gửi `{"id":"x","type":"ping"}` → phải nhận `pong`. Không có `pong` nghĩa là
   tab đó đã mất kết nối (bấm Connect lại).
3. Gọi `GET /api/v1/ws/online` → liệt kê các kết nối đang mở trên instance (tên + số kết nối).
   Thiếu ai thì tab người đó chưa sống.
4. Xem log server sau khi gửi:
   `docker compose -f docker-compose.dev.yml logs app --since 2m | grep "deliver event"`
   → `delivered_connections` là số kết nối đã được đẩy, `not_connected_here` là những người
   không có kết nối nào ở instance này.

Kết nối im lặng quá `WS_READ_TIMEOUT` (mặc định 90s) mà không trả pong sẽ bị server đóng
(log: `dong ket noi WS vi khong co tin hieu song`), close code 1000 nên client chỉ thấy
"disconnected" bình thường.

## 6. Client mẫu (JS)

```js
class ChatSocket {
  constructor(token) { this.token = token; this.pending = new Map(); this.connect(); }

  connect() {
    this.ws = new WebSocket(`wss://host/ws?token=${this.token}`);
    this.ws.onmessage = (e) => this.onFrame(JSON.parse(e.data));
    this.ws.onclose  = () => setTimeout(() => this.connect(), 1000); // reconnect + resync bằng REST
  }

  send(type, payload) {
    const id = crypto.randomUUID();
    this.ws.send(JSON.stringify({ id, type, payload }));
    return id;
  }

  sendMessage(conversationId, content) {
    return this.send('message.send', {
      conversation_id: conversationId,
      content,
      client_msg_id: crypto.randomUUID(),   // gửi lại cùng id khi retry
    });
  }

  onFrame(f) {
    switch (f.type) {
      case 'ready':        break;
      case 'ack':          /* đổi tin nhắn "đang gửi" thành đã gửi theo f.payload.client_msg_id */ break;
      case 'error':        console.warn(f.payload.code, f.payload.message); break;
      case 'message.new':  /* append vào timeline, dedupe theo id + client_msg_id */ break;
      case 'message.read': /* cập nhật tick đã xem */ break;
      case 'typing':       break;
    }
  }
}
```

Sau mỗi lần reconnect, FE nên gọi `GET /api/v1/conversations/:id/messages?limit=50` (và dùng
`meta.next_cursor` để lấy tiếp) để bù những event bị mất trong lúc offline.
