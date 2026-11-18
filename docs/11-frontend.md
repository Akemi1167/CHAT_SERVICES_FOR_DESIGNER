# 11 — Hướng dẫn cho Frontend

Tài liệu dành riêng cho đội làm web chat. Mọi payload trong đây là **response thật** lấy từ service
đang chạy, không phải ví dụ bịa.

- Giao thức realtime chi tiết: [02-websocket.md](02-websocket.md)
- Danh sách endpoint đầy đủ: [05-api.md](05-api.md)
- Hợp đồng với CMS: [10-cms-integration.md](10-cms-integration.md)

---

## 1. Hai chế độ — FE viết code một lần

| | **Fake** (đang chạy, chưa cần CMS) | **CMS** (khi backend CMS sẵn sàng) |
|---|---|---|
| Base URL | `http://localhost:8080` | như cấu hình triển khai |
| Token | `make token SUB=an NAME="An" ROLE=order` | CMS phát: `POST /api/client/v1/workspace/auth/login` |
| Nhóm | FE tự tạo bằng API | CMS tạo sẵn, FE chỉ đọc |
| Card yêu cầu | do Chat dựng (task nội bộ) | do CMS dựng, đã lọc nút theo người xem |

**Đường dẫn và payload giống hệt nhau giữa 2 chế độ**, nên làm UI với chế độ fake trước, lúc đổi sang
CMS chỉ thay nguồn token. Đổi `CMS_ENABLED` là việc của backend.

Hai bộ API cùng tồn tại — chọn **một** bộ rồi dùng xuyên suốt:

| Bộ | Đường dẫn | Envelope | Kiểu tên field | Dùng khi |
|---|---|---|---|---|
| **A** | `/api/v1/*` | `{data, meta}` | `snake_case` | FE tự do chọn; tiện test bằng Postman |
| **B** | `/api/client/v1/chat/*` | `{statusCode, code, message, element}` | `camelCase` | **khớp hợp đồng CMS** — nên dùng nếu sau này đội CMS/FE khác cùng đọc |

Bên dưới ghi cả hai, ký hiệu **(A)** / **(B)**.

---

## 2. Xác thực

Mọi request: `Authorization: Bearer <token>`. WebSocket: `ws://host/ws?token=<token>`
(browser không set được header khi mở WebSocket — **token phải nằm trong URL**).

Token hết hạn/sai → `401`. Chế độ CMS thì FE gọi `POST /workspace/auth/refresh` của CMS lấy token mới
rồi mở lại WebSocket; Chat **không có** login/logout/refresh.

Lấy thông tin mình: **(A)** `GET /api/v1/me` · **(B)** `GET /api/client/v1/chat/auth/me`

```json
{ "data": { "user_id": "416448b2-…", "external_id": "fake-binh",
            "name": "Binh (Design)", "role": "designer" } }
```

`role` dùng để hiển thị: `admin` | `assistant` | `designer` | `order`. **Quyền thao tác luôn do
server quyết định** — đừng ẩn/hiện nút dựa trên role, hãy dựa vào `card.actions` (mục 6).

---

## 3. Quy ước chung

**Phân trang bằng cursor, không dùng offset.** Mọi API list nhận `?limit=30&cursor=…` và trả con trỏ
trang kế tiếp:

Hai bề mặt API có **tên khoá khác nhau** — dùng đúng bề mặt bạn đang gọi:

| Bề mặt | Vỏ response | Con trỏ trang |
|---|---|---|
| `/api/v1/*` (nội bộ, snake_case) | `{data, meta}` | `meta.next_cursor`, `meta.has_more` |
| `/api/client/v1/chat/*` (**hợp đồng CMS, FE dùng cái này**) | `{statusCode, code, message, element}` | `element.pageInfo.nextCursor`, `element.pageInfo.hasMore` |

```json
// /api/v1
{ "data": [ … ], "meta": { "next_cursor": "MTc5NDY1…", "has_more": true } }

// /api/client/v1/chat
{ "statusCode": 200, "code": "SUCCESS", "element": {
    "messages": [ … ], "pageInfo": { "nextCursor": "MTc5NDY1…", "hasMore": true } } }
```

Hết trang khi `hasMore` / `has_more` = `false`. Timeline tin nhắn trả **mới nhất trước**; riêng danh sách phản hồi
(mục 5) trả **cũ → mới** để render thread từ trên xuống.

**Lỗi** luôn có `code` ổn định — bắt theo `code`, đừng so sánh `message`:

```json
{ "error": { "code": "not_conversation_member",
             "message": "Ban khong thuoc cuoc hoi thoai nay",
             "details": null } }
```

| `code` | Nghĩa / xử lý ở FE |
|---|---|
| `unauthenticated` | token sai/hết hạn → làm mới token, mở lại WS |
| `not_conversation_member` | không thuộc nhóm → quay về danh sách nhóm |
| `invalid_uuid`, `invalid_id` | sai id truyền lên |
| `empty_message` | nội dung rỗng và không có tệp |
| `invalid_reply_to_id` / `invalid_thread_id` | tin được trả lời không thuộc nhóm đang mở |
| `action_not_allowed` | người dùng không được phép bấm nút đó trên card |
| `route_not_found` | gọi sai đường dẫn (thường do biến rỗng ghép vào URL) |

**ID**: tất cả là UUID. Riêng nhóm ở chế độ CMS có thêm `chatId` dạng `wg_<hex>` — API nhận **cả hai**,
truyền cái nào cũng được. Nghĩa là `…/chat/groups/:id/messages` thì `:id` =
`wg_95f6fdbfaa17593e` (CMS) **hoặc** `0a142c1f-82c5-…` (UUID nhóm, chế độ fake).

---

## 4. Nhóm

| Việc | (A) | (B) |
|---|---|---|
| Danh sách nhóm + `unread_count` | `GET /api/v1/conversations?limit=30` | `GET …/chat/bootstrap` (kèm `me`, `notifications`) |
| Chi tiết + thành viên | `GET /api/v1/conversations/:id` | có sẵn trong `bootstrap` |
| Tạo nhóm *(chỉ chế độ fake)* | `POST /api/v1/conversations` `{"type":"group","name":"…","member_user_ids":[…]}` | — (CMS tạo) |
| Thêm thành viên *(chỉ fake)* | `POST /api/v1/conversations/:id/members` | — |
| Đồng bộ lại từ CMS | — | `POST …/chat/groups/refresh` |

Một nhóm trả về (rút gọn):

```json
{ "id": "02c255c5-…", "type": "group", "name": "To Design (fake)", "source": "local",
  "last_message_at": "2026-11-18T13:58:32Z", "unread_count": 2,
  "members": [ { "user_id": "291fb0b6-…", "role": "member",
                 "user": { "display_name": "Cuong (Design)", "avatar_url": "",
                           "role": "designer", "presence": "offline" } } ] }
```

`source`: `local` (nhóm tạo trong Chat) hoặc `cms` (CMS quản lý — FE **không** sửa thành viên).
Ở chế độ CMS, mỗi thành viên còn có `sources` (`team`/`assistant`/`manual`/`invite`) và `teamCodes`.

---

## 5. Tin nhắn, reply và thread

### Hai quan hệ khác nhau — đừng gộp làm một

| Trường | Ý nghĩa | UI |
|---|---|---|
| `reply_to_id` / `replyToId` | tin được **trích dẫn** | khung quote nhỏ phía trên bong bóng |
| `thread_id` / `threadRootId` | tin **mở thread** (card yêu cầu) | panel thread bên phải |

Một tin có thể có cả hai: trả lời bạn A **trong** thread của yêu cầu X.

### Đọc tin nhắn

- **(A)** `GET /api/v1/conversations/:id/messages?limit=30&cursor=…`
  - `?only_root=true` — chỉ tin ngoài thread (khung chat chính gọn hơn)
  - `?thread_id=<id>` — tin trong một thread
- **(B)** `GET /api/client/v1/chat/groups/:id/messages?limit=30&threadRootId=…`

Một tin nhắn (A):

```json
{ "id": "5f32cf54-…", "conversation_id": "02c255c5-…",
  "sender_id": "416448b2-…", "sender_type": "user", "type": "text",
  "content": "Mau response", "client_msg_id": "doc-sample-1",
  "reply_count": 0, "thread_count": 0,
  "created_at": "2026-11-18T14:05:10Z",
  "sender": { "display_name": "Binh (Design)", "avatar_url": "", "role": "designer" } }
```

| Field | Dùng để |
|---|---|
| `sender_type` | `user` \| `bot` \| `system` — tin `system`/`bot` render dạng dòng thông báo/card, không có bong bóng |
| `type` | `text` \| `image` \| `video` \| `file` \| `gif` \| `system` \| `bot_command` \| `bot_reply` |
| `reply_count` | số tin **trả lời trực tiếp** → badge nhỏ |
| `thread_count` | số tin **trong thread** tin này mở → nút "N phản hồi" trên card |
| `metadata` | dữ liệu có cấu trúc của tin hệ thống (xem mục 6) |
| `attachments` | `[{kind,url,name,mime_type,size_bytes,width,height,duration_ms}]` |

### Lấy phản hồi của một tin nhắn

**(A)** `GET /api/v1/messages/:message_id/replies?mode=direct|thread&limit=30&cursor=…`
**(B)** `GET /api/client/v1/chat/messages/:id/replies?mode=…`

| `mode` | Trả về | Dùng cho |
|---|---|---|
| `direct` *(mặc định)* | tin có `reply_to_id` = tin này | xem ai trả lời một bong bóng |
| `thread` | tin có `thread_id` = tin này | **panel thread** của card/yêu cầu |

```json
{ "data": { "root": { "id": "…", "content": "Lam giup minh bo hinh su kien thang 11", "thread_count": 2 },
            "replies": [ { "content": "Minh nhan nhe", "sender": {"display_name":"Binh (Design)"} },
                         { "content": "OK deadline 12h", "sender": {"display_name":"An (Order)"} } ],
            "reply_count": 1, "thread_count": 2 },
  "meta": { "has_more": false } }
```

`replies` sắp **cũ → mới**; phân trang tiếp bằng `cursor` như bình thường.

### Gọi thử nhanh bộ (B)

```bash
TOKEN=<token>                     # make seed in ra
GROUP=<chatId hoặc UUID nhóm>

# Lịch sử tin nhắn
curl -s "http://localhost:8080/api/client/v1/chat/groups/$GROUP/messages?limit=20" \
  -H "Authorization: Bearer $TOKEN" | jq

# Gửi tin (kèm trả lời)
curl -s -X POST "http://localhost:8080/api/client/v1/chat/groups/$GROUP/messages" \
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"text":"Gọi API FE thử","replyToId":"<message id>","clientMsgId":"'$(uuidgen)'"}' | jq
```

Response bộ (B) luôn bọc envelope:

```json
{ "statusCode": 200, "code": "SUCCESS", "message": "Success",
  "element": { "duplicated": false,
               "message": { "id": "a0db773b-…", "conversationId": "0a142c1f-…",
                            "text": "Gọi API FE thử", "replyToId": "302804f0-…",
                            "senderType": "user", "createdAt": "2026-11-18T15:01:56Z",
                            "sender": { "name": "An (Order)", "role": "order" } } } }
```

### Gửi tin nhắn

Khuyên dùng WebSocket khi socket đang mở (mục 7), REST làm fallback:

**(A)** `POST /api/v1/conversations/:id/messages`

```json
{ "content": "Mình nhận nhé",
  "reply_to_id": "<id tin trích dẫn>",   // tuỳ chọn
  "thread_id":   "<id tin mở thread>",   // tuỳ chọn
  "type": "image",                        // mặc định text
  "attachments": [{ "kind":"image","url":"https://…","name":"v1.png",
                    "mime_type":"image/png","size_bytes":102400,"width":800,"height":600 }],
  "client_msg_id": "<UUID do FE sinh>" }
```

**(B)** `POST /api/client/v1/chat/groups/:id/messages` — cùng ý nghĩa, camelCase:
`{ "text", "replyToId", "threadRootId", "attachments", "clientMsgId" }`.

> **`client_msg_id` phải duy nhất cho mỗi tin.** Gửi lại đúng giá trị cũ bị coi là retry: server trả
> tin cũ với `duplicated: true` và **không phát cho ai cả**. Sinh `crypto.randomUUID()` mỗi lần bấm
> gửi, chỉ giữ nguyên khi **gửi lại chính tin đó** sau khi mất mạng.

Upload tệp là service khác: FE upload xong lấy URL rồi gửi URL trong `attachments`.

### Các API khác

| Việc | (A) | (B) |
|---|---|---|
| Đánh dấu đã đọc | `POST /api/v1/conversations/:id/read` `{"message_id":"…"}` | `POST …/groups/:id/read` `{"messageId":"…"}` |
| Ai đã xem một tin | `GET /api/v1/messages/:message_id/readers` | — |
| Tìm tin nhắn | `GET /api/v1/messages/search?q=&conversation_id=&sender_id=&from_date=&to_date=` | — |
| Xoá tin (mềm) | `DELETE /api/v1/messages/:message_id` | — |
| Ghim / bỏ ghim | `POST /api/v1/messages/:message_id/pin` (`{"pinned":false}` để bỏ) | — |
| Danh sách ghim | `GET /api/v1/conversations/:id/pinned` | — |
| Thông tin tệp đính kèm | `GET /api/v1/messages/:message_id/attachments/:attachment_id` (`?redirect=true` → 302) | — |
| Thông báo | — | `GET …/chat/notifications`, `POST …/chat/notifications/read` |

---

## 6. Card (yêu cầu / task) — phần quan trọng

Card do backend dựng; **FE chỉ render, không tự suy luận quyền**.

**Chế độ CMS** — tin nhắn có `card` (schema `abc.card/v1`) + `cardVersion` + `actions`:

```json
{ "id": "c38d6afb-…", "senderType": "bot", "cardVersion": 2,
  "card": { "schema": "abc.card/v1",
            "header": { "code": "WEB-20261118-0006", "title": "Banner Tết 2027",
                        "status": { "text": "Đang làm", "tone": "info" } },
            "elements": [ { "type":"button","text":"Nộp bài","action":"submit_work" } ] },
  "actions": ["submit_work"] }
```

- Server **đã lọc** phần tử theo `visibleTo` của chính người đang xem → cứ render hết những gì nhận được.
- `actions` là danh sách action người đó được bấm. Bấm nút:
  `POST /api/client/v1/chat/messages/:id/actions` `{ "action": "submit_work", "values": {…} }`
  → trả `{ ok, toast: {type,text}, closeForm, card, cardVersion }`.
  Hiện `toast`; nếu có `card` mới thì thay card tại chỗ (so `cardVersion` lớn hơn mới thay).
- Người không được phép → `403 action_not_allowed` (server chặn, không phụ thuộc UI).

**Tạo yêu cầu**: `POST /api/client/v1/chat/forms/create_task/submit`

```json
{ "values": { "kind":"task", "title":"Banner Tết 2027", "teamId":"27", "useAI": false },
  "context": { "groupId": "<chatId hoặc id nhóm>" } }
```

`teamId` do FE chọn (danh sách tổ lấy từ CMS). Chat chuyển nguyên sang CMS và trả
`{ ok, toast, closeForm }`; card xuất hiện trong nhóm qua realtime.

**Chế độ fake** — chưa có CMS nên card là **tin hệ thống có `metadata`**:

```json
{ "sender_type": "system", "type": "system",
  "content": "[T-000028] Bo hinh su kien\nTo: To Design · So luong: 2 · …",
  "metadata": { "kind": "task_card", "task_code": "T-000028", "task_status": "waiting_accept",
                "quantity": 2, "quota": "A4",
                "assignments": [ { "display_name": "Designer 1", "status": "pending",
                                   "role": "primary", "assignment_id": "bc7e8b7a-…" } ] } }
```

FE kiểm tra `metadata.kind`: `task_card` → vẽ card, `task_event` → vẽ dòng trạng thái trong thread.
Khi chuyển sang CMS, chỗ này đổi thành `card` + `actions` — **nên tách component render card riêng**
để chỉ phải sửa một chỗ.

---

## 7. WebSocket

```js
const ws = new WebSocket(`ws://localhost:8080/ws?token=${token}`);
```

Nhận `ready` là kết nối thành công — **frame này nói rõ tab đang đăng nhập bằng ai**:

```json
{"type":"ready","payload":{"user_id":"…","display_name":"Binh (Design)","role":"designer","conn_id":"…"}}
```

**Gửi lên** (JSON, mỗi frame `{id, type, payload}`):

| `type` | `payload` |
|---|---|
| `message.send` | `{conversation_id, content, reply_to_id?, thread_id?, attachments?, client_msg_id}` |
| `message.read` | `{conversation_id, message_id}` |
| `message.delete` | `{message_id}` |
| `typing` | `{conversation_id, typing: true}` |
| `template.send` | `{conversation_id, bot_slug, template_code, payload, client_msg_id}` |
| `ping` | — (kiểm tra tab còn sống → nhận `pong`) |

**Nhận về**:

| `type` | Ý nghĩa |
|---|---|
| `ready` | kết nối xong |
| `ack` | request của chính mình xong — `{client_msg_id, message, duplicated}` |
| `error` | `{code, message}` kèm `ref` = `id` của frame đã gửi |
| `message` | tin mới **hoặc** tin được cập nhật (card đổi) → **upsert theo `id`** |
| `message.deleted` | `{conversation_id, message_id}` |
| `read` | ai đó đã đọc tới tin nào |
| `typing` | người khác đang nhập (không gửi lại cho chính mình) |
| `presence` | `{user_id, actor_id, presence: "online"\|"offline"\|"off_duty", reason, online}` — `off_duty` là nghỉ theo lịch (hiện chữ OFF cạnh tên), kèm `reason` |
| `notification` | thông báo riêng cho mình |
| `reload` | cần gọi lại `bootstrap`/danh sách nhóm (được thêm/gỡ khỏi nhóm) |

Luồng gửi lạc quan chuẩn:

```js
function send(conversationId, text, { replyToId, threadRootId } = {}) {
  const clientMsgId = crypto.randomUUID();
  addBubble({ clientMsgId, text, status: 'sending' });          // hiện ngay
  ws.send(JSON.stringify({ id: clientMsgId, type: 'message.send',
    payload: { conversation_id: conversationId, content: text,
               reply_to_id: replyToId, thread_id: threadRootId, client_msg_id: clientMsgId } }));
}

ws.onmessage = (e) => {
  const f = JSON.parse(e.data);
  if (f.type === 'ack')     markSent(f.payload.client_msg_id, f.payload.message);
  if (f.type === 'error')   showError(f.payload.code, f.payload.message);
  if (f.type === 'message') upsertMessage(f.payload);           // dedupe theo id + client_msg_id
  if (f.type === 'read')    updateSeen(f.payload);
  if (f.type === 'reload')  reloadGroups();
};

ws.onclose = () => setTimeout(connect, 1000);                   // reconnect + resync
```

**Sau mỗi lần reconnect phải resync**: WebSocket không phát lại tin cũ. Gọi
`GET …/messages?limit=50` cho nhóm đang mở (và `bootstrap` nếu cần) để bù phần bỏ lỡ.

Kết nối im lặng quá `WS_READ_TIMEOUT` (mặc định 90s) mà client không trả pong sẽ bị đóng với close
code `1000` — trình duyệt tự trả pong nên bình thường không gặp; cứ reconnect trong `onclose`.

---

## 8. Checklist khi làm UI

- [ ] Sinh `client_msg_id` mới cho **mỗi** tin; chỉ dùng lại khi retry đúng tin đó.
- [ ] Dedupe tin đến theo `id`, ghép bong bóng lạc quan theo `client_msg_id`.
- [ ] Phân trang bằng `cursor`, không tự tính offset.
- [ ] Reconnect WebSocket + gọi lại API lịch sử để resync.
- [ ] Phân biệt `reply_to_id` (quote) và `thread_id` (thread); panel thread dùng `?mode=thread`.
- [ ] Badge "N phản hồi" lấy `thread_count`, không đếm ở client.
- [ ] Nút trên card chỉ render từ `card` + `actions` server trả; không tự quyết theo `role`.
- [ ] Bắt lỗi theo `code`; `401` → làm mới token rồi mở lại WebSocket.
- [ ] Tách component render card để chuyển fake → CMS chỉ sửa một chỗ.
- [ ] Nhóm `source: "cms"` thì ẩn các nút sửa thành viên.

## 9. Thử nhanh

```bash
make dev-up                                   # service + DB (localhost:8080)
TOKEN=$(make -s token SUB=an NAME="An" ROLE=order)
curl -s localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
make ws SUB=an NAME=An CONV=<conversation_id> # client WebSocket dòng lệnh
```

Import `postman/chat-service.postman_collection.json` + environment để bấm thử toàn bộ API
(xem [09-postman.md](09-postman.md)).
