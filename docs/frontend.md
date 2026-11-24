# Frontend — hợp đồng cho web chat

> Mới vào dự án? Đọc [FLOW.md](FLOW.md) trước — nó có sơ đồ cả ba bên và lệnh chạy thử
> từng luồng. File này là chi tiết hợp đồng cho FE.

Mọi thứ FE cần: hai bề mặt API khác nhau ở đâu, giao thức WebSocket, thứ tự gọi API
theo từng tình huống, và một giao diện mẫu chạy thật để đối chiếu.

Chi tiết từng endpoint → [backend.md](backend.md)

## Nội dung

- [Hợp đồng FE](#hợp-đồng-fe)
- [Giao thức WebSocket](#giao-thức-websocket)
- [Thứ tự gọi API theo từng tình huống](#thứ-tự-gọi-api-theo-từng-tình-huống)
- [Giao diện mẫu chạy thật](#giao-diện-mẫu-chạy-thật)

---

## Hợp đồng FE

Tài liệu dành riêng cho đội làm web chat. Mọi payload trong đây là **response thật** lấy từ service
đang chạy, không phải ví dụ bịa.

- Giao thức realtime chi tiết: [Giao thức WebSocket](#giao-thức-websocket)
- Danh sách endpoint đầy đủ: [Danh sách API](backend.md#danh-sách-api)
- Hợp đồng với CMS: [Tích hợp CMS (Designer Bot)](backend.md#tích-hợp-cms-designer-bot)

---

### 1. Hai chế độ — FE viết code một lần

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

### 2. Xác thực

Mọi request: `Authorization: Bearer <token>`.
WebSocket: `ws://host/api/client/v1/chat/ws?token=<token>` — **cùng access token với REST**, field trả về camelCase trùng tên REST (`text`, `createdAt`, `threadRootId`)
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

### 3. Quy ước chung

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

### 4. Nhóm

| Việc | (A) | (B) |
|---|---|---|
| Danh sách nhóm + `unread_count` | `GET /api/v1/conversations?limit=30&type=&source=&q=` | `GET …/chat/bootstrap` (kèm `me`, `notifications`) |
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

### 5. Tin nhắn, reply và thread

#### Hai quan hệ khác nhau — đừng gộp làm một

| Trường | Ý nghĩa | UI |
|---|---|---|
| `reply_to_id` / `replyToId` | tin được **trích dẫn** | khung quote nhỏ phía trên bong bóng |
| `thread_id` / `threadRootId` | tin **mở thread** (card yêu cầu) | panel thread bên phải |

Một tin có thể có cả hai: trả lời bạn A **trong** thread của yêu cầu X.

#### Đọc tin nhắn

- **(A)** `GET /api/v1/conversations/:id/messages?limit=30&cursor=…`
  - `?only_root=true` — chỉ tin ngoài thread (khung chat chính gọn hơn)
  - `?thread_id=<id>` — tin trong một thread
- **(B)** `GET /api/client/v1/chat/groups/:id/messages?limit=30&threadRootId=…`
  - **mặc định chỉ trả tin gốc** — tin trả lời trong thread **không** nằm trong lịch sử nhóm,
    mà gom vào tin gốc qua `threadCount`
  - `?threadRootId=<id>` — bấm vào tin gốc: trả tin gốc + toàn bộ tin trong thread đó
    (hoặc `GET …/messages/:id/replies?mode=thread`, trả `root` + `replies` cũ → mới)
  - `?includeThread=true` — lấy cả tin trong thread lẫn vào lịch sử (hành vi cũ, để debug)

**Realtime với thread**: event `message` của tin trả lời trong thread có `threadRootId`. FE
**không** chèn tin đó vào khung chat chính — tăng `threadCount` của tin gốc (badge "N trả lời"),
và nếu panel thread đó đang mở thì upsert vào panel.

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

#### Lấy phản hồi của một tin nhắn

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

#### Panel chi tiết một đơn theo mã — `GET /api/client/v1/chat/cards/:code`

Khi chỉ có **mã đơn** (`header.code`, vd `WEB-20261123-0001` — từ thông báo, link, ô tìm kiếm)
mà không có `messageId`. Một request trả đủ để vẽ panel: card + khối Drive + thread đã tách sẵn.

```json
{ "statusCode": 200, "code": "SUCCESS", "element": {
    "card":  { "id": "3a0e…", "type": "bot_command", "card": { … }, "cardVersion": 2, "actions": ["accept_task"] },
    "root":  { …cùng tin với card; khác khi :code là card con -P1/-E1 nằm trong thread… },
    "links": [ { "type": "links", "title": "Google Drive", "items": [ { "label": "Folder đơn", "url": "https://drive…" }, … ] } ],
    "activity":   [ { "senderType": "bot", "text": "📁 **Google Drive** (bấm link để mở)\n• …", "threadRootId": "3a0e…" } ],
    "discussion": [],
    "threadCount": 1,
    "pageInfo": { "hasMore": false, "nextCursor": "" } } }
```

| Field | Ý nghĩa |
|---|---|
| `card` | tin chứa card mới nhất có mã này — **đã lọc `visibleTo`**, `actions` = nút người xem được bấm |
| `links` | các phần tử `{"type":"links"}` của card (vd "Google Drive"). `[]` = CMS chưa gửi (đang tạo folder / đơn không có Drive) |
| `activity` | dòng diễn biến bot / hệ thống trong thread, cũ → mới (vd "Đã giao…", tin Drive, "SKY · Nhận task") |
| `discussion` | tin **người dùng** trao đổi trong thread, cũ → mới. Rỗng → hiện "Chưa có trao đổi" |
| `threadCount` | tổng số tin trong thread (cả hai loại) |

Phân trang thread bằng `?cursor=&limit=` (tối đa 100) — mỗi trang tách lại `activity` / `discussion`.
Lỗi: mã không tồn tại → `404 CARD_NOT_FOUND`; không thuộc nhóm chứa card → `403 NOT_CONVERSATION_MEMBER`.

#### Gọi thử nhanh bộ (B)

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

#### Gửi tin nhắn

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

#### Các API khác

| Việc | (A) | (B) |
|---|---|---|
| Đánh dấu đã đọc | `POST /api/v1/conversations/:id/read` `{"message_id":"…"}` | `POST …/groups/:id/read` `{"messageId":"…"}` |
| Ai đã xem một tin | `GET /api/v1/messages/:message_id/readers` | — |
| Tìm tin nhắn | `GET /api/v1/messages/search?q=&conversation_id=&sender_id=&from_date=&to_date=` | — |
| Xoá tin (mềm) | `DELETE /api/v1/messages/:message_id` | — |
| Ghim / bỏ ghim | `POST /api/v1/messages/:message_id/pin` (`{"pinned":false}` để bỏ) | — |
| Danh sách ghim | `GET /api/v1/conversations/:id/pinned` | — |
| Thông tin tệp đính kèm | `GET /api/v1/messages/:message_id/attachments/:attachment_id` (`?redirect=true` → 302) | — |
| Thông báo | — | `GET …/chat/notifications?unread=true&cursor=&limit=`, `POST …/chat/notifications/read` — mỗi thông báo cùng hình dạng với frame `notification` (camelCase) |
| Realtime | `ws://…/ws?token=` | `ws://…/api/client/v1/chat/ws?token=` — **cùng access token, field trùng tên với REST** |

---

### 6. Card (yêu cầu / task) — phần quan trọng

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
- CMS trả `ok: true` → server ghi thêm 1 tin `type: "system"` vào **thread của card**
  (`text` = `"<tên> · <chữ trên nút>"`, `metadata.kind = "card_action"`, kèm `action`, `values`,
  `card_message_id`). Tin này tới qua realtime như tin thường — FE không phải tự thêm.
- Người không được phép → `403 action_not_allowed` (server chặn, không phụ thuộc UI).

**Tạo yêu cầu**: `POST /api/client/v1/chat/forms/create_task/submit`

```json
{ "values": { "kind":"task", "title":"Banner Tết 2027", "teamId":"9", "useAI": false },
  "context": { "groupId": "<chatId hoặc id nhóm>" } }
```

CMS trả `ok: true` → server ghi 1 tin `type: "system"` vào nhóm (vào thread nếu gửi
`context.threadRootId`): `text` = `"<tên> · <toast.text>"`, `metadata.kind = "form_submitted"`,
kèm `form_id`, `values`, `toast`. Card đơn vẫn tới **sau**, khi CMS gọi `POST /bot/v1/messages`.

`teamId` **phải là tổ thuộc nhóm đang mở** — lấy từ `bootstrap` → `groups[].cms.teams[].id`,
đừng hardcode. Gửi tổ không thuộc nhóm thì CMS từ chối:

```json
{ "statusCode": 200, "element": {
    "ok": true,
    "toast": { "type": "error", "text": "Chọn tổ thực hiện thuộc nhóm này" },
    "closeForm": false } }
```

⚠️ `statusCode: 200` và `ok: true` **không** có nghĩa là tạo được đơn — chúng chỉ nói "Chat
chuyển sang CMS được" và "CMS xử lý được request". Kết quả nghiệp vụ nằm ở `toast.type` và
`closeForm`:

```js
if (el.toast) toast(el.toast.text, el.toast.type);
if (el.closeForm) closeForm();        // closeForm:false = chưa xong, giữ form để sửa
```

Chat **không đọc `values`** nên không chặn lỗi này sớm hơn được — muốn tránh thì FE chỉ cho chọn
trong `groups[].cms.teams`.

Card đơn **không** đến từ response này. Nó xuất hiện khi CMS gọi ngược vào Chat
(`POST /bot/v1/messages`) — xem luồng E bên dưới.

**Chế độ fake** — chưa có CMS nên card là **tin hệ thống có `metadata`**:

```json
{ "sender_type": "system", "type": "system",
  "content": "[T-000028] Bo hinh su kien\nTo: To Design · So luong: 2 · …",
  "metadata": { "kind": "task_card", "task_code": "T-000028", "task_status": "waiting_accept",
                "quantity": 2, "quota": "A4",
                "assignments": [ { "display_name": "Designer 1", "status": "pending",
                                   "role": "primary", "assignment_id": "bc7e8b7a-…" } ] } }
```

**Khối Google Drive** (`{"type":"links","title":…,"items":[{label,url}]}`): mở link ở tab mới.
Card mới tạo **chưa có** khối này — vài giây sau CMS `PATCH` card lên `cardVersion` mới có `links`
và gửi thêm một tin markdown vào thread. Cả hai tới qua realtime `message`; FE chỉ cần upsert theo
`id` (version lớn hơn mới thay) là card tự hiện khối Drive. Chi tiết: [FLOW.md › G1](FLOW.md#g1-link-google-drive-của-đơn).

**Tin trong thread là markdown** (`**đậm**`, `[chữ](url)`) và xuống dòng bằng `\n` đơn. Markdown
chuẩn coi `\n` đơn là dấu cách → cả tin dính thành một dòng. Bật chế độ xuống dòng cứng
(vd `remark-breaks`, `marked` với `breaks: true`).

FE kiểm tra `metadata.kind`: `task_card` → vẽ card, `task_event` → vẽ dòng trạng thái trong thread.
Khi chuyển sang CMS, chỗ này đổi thành `card` + `actions` — **nên tách component render card riêng**
để chỉ phải sửa một chỗ.

---

### 7. WebSocket

```js
const ws = new WebSocket(`ws://localhost:8080/api/client/v1/chat/ws?token=${token}`);
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
| `notification` | thông báo riêng cho mình: `{id, text, read, createdAt, messageId, conversationId, groupId, threadRootId}` — có `threadRootId` thì mở thread đó rồi cuộn tới `messageId` (card của một phần khi chia task) |
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

### 8. Checklist khi làm UI

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

### 9. Thử nhanh

```bash
make dev-up                                   # service + DB (localhost:8080)
TOKEN=$(make -s token SUB=an NAME="An" ROLE=order)
curl -s localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN"
make ws SUB=an NAME=An CONV=<conversation_id> # client WebSocket dòng lệnh
```

Import `internal/postman/chat-service.postman_collection.json` + environment để bấm thử toàn bộ API
(xem [Postman](backend.md#postman)).

---

## Giao thức WebSocket

> Frame **`presence.list`** để hỏi *ai đang online* (ảnh chụp hiện tại) — chi tiết ở [FLOW.md](FLOW.md#41-frame-fe-gửi-lên-websocket--7-loại).

Có **hai đường vào**, cùng một hub, cùng một event — chỉ khác hình dạng JSON:

| | `/ws` | `/api/client/v1/chat/ws` |
|---|---|---|
| Dùng cho | công cụ nội bộ (`make ws`, Postman, service khác) | **FE theo hợp đồng CMS** |
| Token | JWT nào service xác thực được | **chính access token của CMS** (giống REST `/api/client/v1/chat/*`) |
| Envelope | `event_id`, `conversation_id` | `eventId`, `conversationId` |
| Nội dung tin | `content` | `text` |
| Thread | `thread_id` | `threadRootId` |
| `sender` | cả `external_id`, `status`, `synced_at`, `is_designer`… | chỉ trường UI cần |
| Frame `ready` | `conn_id`, `display_name`, `external_id` | `connId`, `name`, `actorId` |

```
GET ws://HOST/ws?token=<JWT>                       # bản gốc, snake_case
GET ws://HOST/api/client/v1/chat/ws?token=<JWT>    # hợp đồng CMS, camelCase
```

FE **không có token riêng cho WebSocket**: dùng lại đúng access token đang gọi REST.
Nhờ đường thứ hai, field đọc được từ WebSocket trùng tên với field REST trả về — FE
không phải viết hàm đổi tên khoá.

Hình dạng này do `internal/transport/httpserver/clientview` dựng, và **REST cũng gọi
đúng package đó**, nên hai đường không thể lệch nhau.

Server nhận token theo thứ tự **`?token=` → `Sec-WebSocket-Protocol` → header `Authorization`**
(REST thì ngược lại: header trước). Lý do ưu tiên query ở WebSocket: browser không set được header
khi mở WebSocket, và các tool hay tự gắn header `Authorization` kế thừa (ví dụ request WebSocket
lưu trong Postman collection có Bearer auth) — nếu header thắng thì tab sẽ đăng nhập bằng **người
khác**, vẫn nhận `ready` nhưng không bao giờ nhận tin của room.

Frame `ready` trả về `user_id`, `external_id`, `display_name`, `role` — luôn kiểm tra frame này để
biết tab đang là ai.

```js
const ws = new WebSocket(`wss://host/api/client/v1/chat/ws?token=${jwt}`);
```

Token sai/hết hạn → server trả frame `error` rồi đóng kết nối (close code chuẩn).
Token hợp lệ → user được upsert vào bảng `users` (theo claim `sub`), presence = `online`,
và server gửi ngay frame `ready`.

Claim được dùng: `sub` (bắt buộc, = external user id), `name`, `email`, `avatar`, `role`, `tenant_id`.

### 2. Dạng frame

Mọi frame là một JSON object.

Client → Server:
```json
{ "id": "<id request do FE tự sinh>", "type": "<loại>", "payload": { } }
```

Server → Client:
```json
{ "type": "<loại>", "ref": "<id request nếu là ack/error>", "payload": { }, "ts": "..." }
```

### 3. Frame client gửi lên

#### `message.send` — gửi tin nhắn (text/ảnh/video/file, reply, thread, và cả `@bot`)

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

#### `template.send` — gửi mẫu cho bot từ form (không cần gõ `@bot`)
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

#### `message.read` — đánh dấu đã đọc
```json
{"id":"c3","type":"message.read","payload":{"conversation_id":"uuid","message_id":"uuid"}}
```

#### `message.delete` — xoá mềm (người gửi hoặc admin/owner nhóm)
```json
{"id":"c4","type":"message.delete","payload":{"message_id":"uuid"}}
```

#### `typing` — đang nhập (không lưu DB, không ack)
```json
{"id":"c5","type":"typing","payload":{"conversation_id":"uuid","typing":true}}
```

#### `ping` — giữ kết nối / đo RTT (server cũng tự ping ở tầng protocol mỗi `WS_PING_INTERVAL`)
```json
{"id":"c6","type":"ping"}
```

### 4. Frame server gửi xuống

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
| `presence` | user đổi trạng thái (mở/đóng WebSocket, hoặc CMS báo lịch OFF) | `{user_id, actor_id, presence: "online"\|"offline"\|"off_duty", reason, online}` — chỉ gửi cho người **chung hội thoại** |
| `conversation.created` / `conversation.updated` | được thêm vào nhóm, nhóm đổi thành viên | object conversation |
| `bot.submission.stored` | bot đã lưu 1 mẫu | object submission |
| `task.created` | có yêu cầu/task mới trong hội thoại | `{task}` |
| `task.updated` | task đổi trạng thái, đổi phân công, có phiếu sửa... | `{task, event_type, ...}` — vẽ lại card từ `task` |

Mọi diễn biến task cũng đến dưới dạng `message.new` (tin nhắn hệ thống, `metadata.kind` =
`task_card` \| `task_event`) để hiển thị trong dòng chat.

Mã lỗi hay gặp: `unauthenticated`, `not_conversation_member`, `conversation_not_found`,
`invalid_uuid`, `empty_message`, `bot_template_not_found`, `bot_template_missing_fields`.

### 5. Cách test nhanh (nhiều người cùng lúc)

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

**Cách 2 — Postman.** New → WebSocket → dán `ws://localhost:8080/api/client/v1/chat/ws?token=<JWT>` → **Connect**.
Muốn xem đúng hình dạng FE nhận được thì dùng request *Chat realtime — bề mặt FE (hợp đồng CMS)*
trong collection (URL `{{ws_client_url}}`, token `{{cms_token}}`).
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

### 6. Client mẫu (JS)

```js
class ChatSocket {
  constructor(token) { this.token = token; this.pending = new Map(); this.connect(); }

  connect() {
    this.ws = new WebSocket(`wss://host/api/client/v1/chat/ws?token=${this.token}`);
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

---

## Thứ tự gọi API theo từng tình huống

Mỗi mục là một kịch bản: **ai gọi gì, theo thứ tự nào, nhận lại gì, và event realtime nào bắn ra**.
Ký hiệu: **FE** = web chat · **CHAT** = service này · **CMS** = Task Engine.

---

### A. Mở app (lần đầu / sau khi mất phiên)

```
FE ──1──► CMS   POST /api/client/v1/workspace/auth/login {identifier, password}
   ◄─────       200 {accessToken, refreshToken, user}

FE ──2──► CHAT  GET /api/client/v1/chat/bootstrap          Bearer <accessToken>
   ◄─────       {me, groups[], notifications[], unreadNotifications}
                  (lần đầu CHAT tự: kiểm token bằng JWKS của CMS → tạo actor
                   → GET /workspace/users?ids= lấy hồ sơ)

FE ──3──► CHAT  WebSocket  ws(s)://…/api/client/v1/chat/ws?token=<accessToken>
   ◄─────       frame {"type":"ready","payload":{userId, actorId, name, role, connId, serverNow}}
```

| # | Gọi | Dùng để |
|---|---|---|
| 1 | CMS `auth/login` | lấy token (Chat **không** có login) |
| 2 | CHAT `bootstrap` | danh sách nhóm + `unreadCount` + thông báo → vẽ sidebar |
| 3 | CHAT `/api/client/v1/chat/ws` | mở realtime, giữ suốt phiên |

Token hết hạn (401 ở bất kỳ request nào) → FE gọi CMS `POST /workspace/auth/refresh`, rồi **mở lại WebSocket** bằng token mới.

---

### B. Mở một nhóm

```
FE ──1──► CHAT  GET  /api/client/v1/chat/groups/{id}/messages?limit=30
   ◄─────       {messages[] (mới→cũ, card đã lọc visibleTo), pageInfo}

FE ──2──► CHAT  POST /api/client/v1/chat/groups/{id}/read  {"messageId": "<tin mới nhất>"}
   ◄─────       {unreadCount: 0}
          CHAT ══► realtime "read" cho cả nhóm  (FE khác cập nhật dấu đã xem)

FE ──3──► CHAT  GET  /api/client/v1/chat/groups/{id}/messages?cursor=<pageInfo.nextCursor>   (cuộn lên)
```

`{id}` nhận **cả** `wg_…` (CMS) lẫn UUID nhóm. Mỗi tin có `replyCount`, `threadCount` để vẽ badge.

---

### C. Gửi tin nhắn thường

```
FE ──1──► CHAT  (WebSocket) {"id":"<uuid>","type":"message.send",
                              "payload":{conversation_id, content, client_msg_id}}
   ◄─────       {"type":"ack","ref":"<uuid>","payload":{message, duplicated}}
          CHAT ══► realtime "message" cho mọi thành viên (kể cả người gửi, thiết bị khác)
          CHAT ──► CMS  webhook message.created        (bất đồng bộ, qua outbox, retry backoff)
```

Không có WebSocket thì dùng REST, payload tương đương:
`POST /api/client/v1/chat/groups/{id}/messages {text, clientMsgId}`.

> `client_msg_id` phải **duy nhất mỗi tin**. Gửi lại đúng id cũ → `duplicated: true`, **không**
> broadcast (xem [Hợp đồng FE](#hợp-đồng-fe) §5).

---

### D. Trả lời & thread

```
FE ──1──► CHAT  WS message.send  payload {conversation_id, content,
                                          reply_to_id: <tin trích dẫn>,
                                          thread_id:  <tin mở thread>}
FE ──2──► CHAT  GET /api/client/v1/chat/messages/{id}/replies?mode=thread   (mở panel thread)
   ◄─────       {root, replies[] (cũ→mới), replyCount, threadCount}
FE ──3──► CHAT  GET …/replies?mode=direct                                  (xem ai trả lời 1 bong bóng)
```

---

### E. Tạo yêu cầu (đơn) — vòng tròn đầy đủ

```
FE  ──1──► CMS   GET /workspace/teams… (chọn tổ, gợi ý người nhận)   [API của CMS]
FE  ──2──► CHAT  POST /api/client/v1/chat/forms/create_task/submit
                   {values:{kind,title,brief,teamId,useAI,assigneeOpenId}, context:{groupId}}
           CHAT ──3──► CMS   webhook form.submit  (ký HMAC, chờ tối đa 5s)
                 CMS tạo đơn WEB-YYYYMMDD-NNNN, chọn người nhận / xếp hàng
                 CMS ──4──► CHAT  POST /bot/v1/users/resolve   {items:[{openId}]} → actorId
                 CMS ──5──► CHAT  POST /bot/v1/messages        {groupId, bot, card} → {messageId, cardVersion:1}
                              CHAT ══► realtime "message" (card đã lọc visibleTo cho TỪNG người)
                 CMS ──6──► CHAT  POST /bot/v1/messages/{id}/thread {text:"Đã giao … · Chờ nhận task."}
                 CMS ──7──► CHAT  POST /bot/v1/notifications  {actorId, text, messageId}
                              CHAT ══► realtime "notification" cho đúng người nhận
           CHAT ◄──8── CMS   200 {ok, toast:{type,text}, closeForm}
FE  ◄──9── CHAT  {statusCode:200, element:{ok, toast, closeForm}}   → hiện toast, đóng form
```

FE chỉ gọi **một** request (bước 2); các bước 3–8 là Chat ↔ CMS.

⚠️ **Ở local, bước 5–7 không xảy ra.** Chat chạy ở `localhost:8080` nên CMS (ở Internet) không
gọi tới được — submit thành công nhưng **card không hiện trong nhóm**. Chiều Chat → CMS chạy
bình thường vì máy bạn gọi ra được.

| Chiều | Local | Vì sao |
|---|---|---|
| Chat → CMS (`form.submit`, `card.action`) | chạy | máy bạn gọi ra Internet được |
| CMS → Chat (`POST /bot/v1/messages`) | **không** | CMS không thấy `localhost` của bạn |

Muốn thông: deploy Chat lên domain công khai rồi đưa CMS `CHAT_API_BASE` + `CHAT_SERVICE_KEY`
(xem [cutover](deploy.md#cutover-bật-tích-hợp-cms)), hoặc tạm mở tunnel
(`cloudflared tunnel --url http://localhost:8080`).

Kiểm CMS có gọi vào chưa:

```bash
make log-grep Q='"path":"/bot/v1/messages"'
```

---

### F. Bấm nút trên card

```
FE ──1──► CHAT  POST /api/client/v1/chat/messages/{messageId}/actions
                  {action:"accept_task", values:{normItemId, quantity}}

          CHAT kiểm: action có trong card? actor ∈ visibleTo?
               không → 403 ACTION_NOT_ALLOWED (dừng, KHÔNG gọi CMS)

          CHAT ──2──► CMS  webhook card.action {messageId, action, values, actor, group, cardVersion}
                CMS cập nhật đơn (khoá lạc quan theo cardVersion)
                CMS ──3──► CHAT  POST /bot/v1/messages/{id}/thread   (dòng hệ thống)
                CMS ──4──► CHAT  POST /bot/v1/notifications          (nếu cần)
          CHAT ◄──5── CMS  200 {ok, toast, closeForm, card, cardVersion:2}

          CHAT thay card nếu cardVersion LỚN HƠN bản đang lưu
               ══► realtime "message" (mỗi người một bản card đã lọc)
FE ◄──6── CHAT  {ok, toast, closeForm, card (đã lọc), cardVersion}
```

Lỗi nghiệp vụ: CMS vẫn trả **200** nhưng `toast.type:"error"`, `closeForm:false`, **không có `card`** →
FE chỉ hiện toast, giữ nguyên card cũ.

---

### G. CMS chủ động (tự giao, đổi định mức, nhắc việc)

Không có FE trong luồng này — FE chỉ nhận realtime:

```
CMS ──► CHAT  PATCH /bot/v1/messages/{id}          ← MỘT request, ba việc
                {card, cardVersion:3,
                 thread: {text:"Tự giao … · Chờ nhận task."},
                 notify: [{actorId:"u_68", text:"Bạn có task mới"}]}
              → {ok, cardVersion:3,
                 thread:{messageId}, notify:[{id, actorId}]}

                 cardVersion ≤ bản đang lưu → {ok, ignored:true}
                 và BỎ QUA cả thread/notify (đang gửi trạng thái cũ)

        CHAT ══► realtime "message" + "notification"
```

---

### H. Thành viên nhóm thay đổi (thêm tay / link mời / đổi tổ)

```
CMS ──► CHAT  PUT /bot/v1/groups/{chatId} {name, members[]}   → {id, memberCount}
        CHAT đặt danh sách thành viên = ĐÚNG danh sách CMS gửi (ai mất khỏi list → mất quyền xem)
        CHAT ══► realtime "reload" cho người vừa vào + người vừa bị gỡ
FE  ──► CHAT  GET /api/client/v1/chat/bootstrap         (nhận "reload" thì tải lại danh sách nhóm)
```

Vào nhóm bằng link mời: FE gọi CMS `POST /workspace/groups/join {token}` trước, rồi
`POST /api/client/v1/chat/groups/refresh` để Chat hỏi lại CMS ngay (không phải chờ chu kỳ 60s).

---

### I. Các luồng phụ

| Việc | FE gọi | Realtime bắn ra |
|---|---|---|
| Đang nhập | WS `{"type":"typing","payload":{conversation_id, typing:true}}` | `typing` (không gửi lại cho chính mình) |
| Online/OFF | tự động khi mở/đóng WS; CMS đẩy lịch OFF qua `PUT /bot/v1/presence` | `presence` cho người **chung nhóm** |
| Ghim tin | `POST /api/v1/messages/{id}/pin` (hoặc CMS `POST /bot/v1/messages/{id}/pin`) | `message` (tin đã cập nhật) |
| Xem tệp | `GET /api/v1/messages/{mid}/attachments/{aid}` (`?redirect=true` → 302) | — |
| Thông báo | `GET …/chat/notifications`, `POST …/chat/notifications/read` | `notification` |

---

### J. Bảng tra nhanh — FE gọi gì, khi nào

| Thời điểm | Request |
|---|---|
| Mở app | CMS `auth/login` → CHAT `GET /chat/bootstrap` → mở `/ws` |
| Mở nhóm | `GET /chat/groups/{id}/messages` → `POST /chat/groups/{id}/read` |
| Cuộn lên | `GET …/messages?cursor=<nextCursor>` |
| Gửi tin | WS `message.send` (fallback `POST /chat/groups/{id}/messages`) |
| Mở thread | `GET /chat/messages/{id}/replies?mode=thread` |
| Mở chi tiết đơn theo mã | `GET /chat/cards/{code}` → card + `links` (Drive) + `activity` + `discussion` |
| Tạo yêu cầu | `POST /chat/forms/create_task/submit` |
| Bấm nút card | `POST /chat/messages/{id}/actions` |
| Nhận `reload` | `GET /chat/bootstrap` lại |
| 401 | CMS `auth/refresh` → mở lại `/ws` |

---

### K. Tự test luồng ngay trên máy

```bash
make up                      # Chat + Postgres + Redis
make cms-check               # phải 6/6 OK (4 đường nối CMS)
TOK=$(make token-cms CMS_USER=6 | tail -1)    # token dev đóng vai user CMS id 6

curl -s localhost:8080/api/client/v1/chat/bootstrap -H "Authorization: Bearer $TOK"
curl -s -X POST localhost:8080/api/client/v1/chat/groups/wg_xxx/messages \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"text":"hello","clientMsgId":"'$(uuidgen)'"}'

# event đã sang CMS chưa:
docker compose -f docker-compose.dev.yml exec postgres \
  psql -U chat -d chat -c "SELECT event_type,status,attempts FROM webhook_outbox ORDER BY created_at DESC LIMIT 5;"
```

`status=sent` là CMS đã nhận. `make token-cms` **chỉ chạy được ở local** (`AUTH_ALLOW_DEV_TOKEN=true`);
trên production FE bắt buộc dùng token thật của CMS.

**Đóng vai CMS để thấy card hiện ra** (vì CMS thật không gọi được vào `localhost`):

```bash
KEY=$(grep '^CHAT_SERVICE_KEY=' .env | cut -d= -f2 | tr -d ' ')
curl -s -X POST localhost:8080/bot/v1/messages \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d '{"groupId":"wg_xxx","bot":{"id":1,"name":"Designer Bot"},
       "card":{"schema":"abc.card/v1","header":{"title":"Đơn thử"},
               "elements":[{"type":"button","text":"Nhận việc","action":"accept_task"}]}}'
```

Card hiện ngay trong `/demo/live.html`. ⚠️ Khoá của nút là **`action`**, không phải `actionId` —
sai tên thì bấm nút trả `403 action_not_allowed`.

Giao diện thử tay: `http://localhost:8080/demo/live.html`. Realtime bằng dòng lệnh:
`make ws TOKEN=$TOK`. Chạy cả luồng bằng Postman: `make postman-flow TOKEN=$TOK KEY=$KEY`.

---

Danh sách endpoint đầy đủ: [Danh sách API](backend.md#danh-sách-api) · chi tiết cho FE: [Hợp đồng FE](#hợp-đồng-fe) ·
hợp đồng với CMS: [Tích hợp CMS (Designer Bot)](backend.md#tích-hợp-cms-designer-bot).

---

## Giao diện mẫu chạy thật

Bản chạy thật của `chat-thread-demo.html`: **giữ nguyên toàn bộ CSS và khung HTML của demo**,
nhưng mọi dữ liệu lấy từ API thật thay vì IndexedDB.

```
http://localhost:8080/demo/live.html
```

### Mở lên

```bash
make up                                   # Chat + Postgres + Redis
TOK=$(make token-cms CMS_USER=6 | tail -1)   # token dev đóng vai user CMS
echo "$TOK"                               # dán vào ô token trên trang
open http://localhost:8080/demo/live.html
```

Trang phục vụ **cùng origin** với API nên không vướng CORS và WebSocket nối thẳng được.
Token lưu trong `localStorage` (bỏ tick "Ghi nhớ" nếu không muốn). Ở production tắt bằng
`WEB_DEMO_DIR=""` — `docker-compose.prod.yml` đã đặt sẵn.

### Đã nối những gì

| Thao tác trên giao diện | API gọi |
|---|---|
| Mở trang | `GET /chat/bootstrap` → danh sách nhóm, hồ sơ, thông báo |
| (ngay sau đó) | `WS /api/client/v1/chat/ws?token=…` → realtime |
| Bấm một nhóm | `GET /chat/groups/{id}/messages?limit=40` |
| Cuộn lên đầu | `GET …/messages?cursor=…` (tự động) |
| Nhìn thấy tin mới | `POST /chat/groups/{id}/read` |
| Gõ Enter | WS `message.send` (WS đứt → `POST …/messages`) |
| Đang gõ | WS `typing` → người khác thấy "… đang nhập" ở tiêu đề |
| Bấm "N phản hồi" | `GET /chat/messages/{id}/replies?mode=thread` |
| Trả lời trong thread | WS `message.send` kèm `thread_id` |
| Bấm nút trên card | `POST /chat/messages/{id}/actions` |
| "Tạo yêu cầu" | `POST /chat/forms/create_task/submit` |
| Nút bút chì (sidebar) | `POST /chat/groups/refresh` — hỏi lại CMS ngay |
| Nút thông tin (header) | thành viên + thông báo của nhóm |

Realtime xử lý đủ: `message`, `message.deleted`, `read`, `typing`, `presence`, `notification`,
`reload` (tự `bootstrap` lại). Mất kết nối thì tự nối lại với backoff, và nhãn ở góc trái dưới
chuyển sang **mất kết nối**.

### Hai điểm kỹ thuật đáng nhớ

**REST trả camelCase, WebSocket trả snake_case.** `GET /messages` cho `{text, senderId, createdAt}`
còn event realtime đẩy thẳng `domain.Message` nên là `{content, sender_id, created_at}`.
`normalize()` trong `web/live.js` gom hai dạng về một. FE thật cũng phải làm vậy.

**Card do CMS dựng, Chat không hiểu nội dung.** Bộ render card (`cardElementHTML`) đi theo kiểu
"biết gì vẽ nấy": `text`, `note`, `divider`, `field/fields`, `button`, `actions/row/group/section`,
`list`. Gặp `type` lạ thì vẫn in chữ ra chứ không bỏ im lặng. Khi CMS gửi tài liệu
`CHAT_SERVICE_UI_API.md` mục 2 (từ vựng card đầy đủ) thì bổ sung thêm case vào đúng hàm đó.

### Màn hình của demo bị ẩn

"Đơn hàng", "Tệp", "Work item", "Phiếu sửa", "Tổ" — **CMS sở hữu dữ liệu này**, Chat không có API
cho chúng. Khung HTML vẫn nằm trong trang (đã ẩn bằng `#live-overrides`), nên khi CMS mở API thì
bật lại chỉ là chuyện nối dữ liệu, không phải dựng lại giao diện.

### Đã kiểm chứng trên trình duyệt

Mở Chrome với nhóm thật **TEAM TEST** của CMS:

- đăng nhập bằng token → sidebar 1 nhóm, tiêu đề `4 thành viên · 1 đang online · DEV-TEST`;
- 15 tin nhắn lịch sử render đúng, có nhãn ngày "Hôm qua"/"Hôm nay";
- card thật `WEB-20261118-0006 · Đang làm · Banner Tết 2027` hiện kèm nút `submit_work`;
- gõ Enter → tin xuất hiện, WS ack;
- mở thread → bố cục chuyển sang 3 cột, panel phải hiện card gốc + phản hồi;
- mở tab thứ hai bằng token của `tester1`, gửi một tin → tab thứ nhất nhận **tức thì**, hiển thị
  là tin đến và preview ở sidebar đổi theo.
