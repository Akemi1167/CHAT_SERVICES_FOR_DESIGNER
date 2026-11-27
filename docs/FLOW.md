# FLOW — toàn bộ luồng FE ↔ Chat ↔ CMS

Một file duy nhất cho **cả ba đội**: FE, Chat (service này), CMS. Mỗi luồng có sơ đồ, API kèm
tham số, response thật, và **lệnh chạy thử dán là chạy**.

> Mọi khối ```` ``` ```` trong file này đều có nút copy sẵn khi xem trên GitHub, VS Code hoặc
> IDE JetBrains. Dữ liệu trong lệnh là **dữ liệu thật** của workspace đang chạy (`wg_00f2…`,
> `teamId=9`, `u_6`) — không phải placeholder.

## Nội dung

- [1. Ba bên, ai sở hữu cái gì](#1-ba-bên-ai-sở-hữu-cái-gì)
- [2. Ba bề mặt API](#2-ba-bề-mặt-api)
- [3. Chuẩn bị để chạy thử](#3-chuẩn-bị-để-chạy-thử)
- [4. Danh mục đầy đủ các loại "type"](#4-danh-mục-đầy-đủ-các-loại-type)
- [5. Khi nào CMS cần gọi sang Chat](#5-khi-nào-cms-cần-gọi-sang-chat)
- [Luồng A — Mở app](#luồng-a--mở-app)
- [Luồng B — Mở một nhóm, đọc tin](#luồng-b--mở-một-nhóm-đọc-tin)
- [Luồng C — Gửi tin nhắn](#luồng-c--gửi-tin-nhắn)
- [Luồng D — Trả lời & thread](#luồng-d--trả-lời--thread)
- [Luồng E — Tạo đơn (vòng tròn đầy đủ)](#luồng-e--tạo-đơn-vòng-tròn-đầy-đủ)
- [Luồng F — Bấm nút trên card](#luồng-f--bấm-nút-trên-card)
- [Luồng G — CMS cập nhật đơn](#luồng-g--cms-cập-nhật-đơn)
  - [G1. Link Google Drive của đơn](#g1-link-google-drive-của-đơn)
- [Luồng H — Thành viên nhóm thay đổi](#luồng-h--thành-viên-nhóm-thay-đổi)
- [Luồng I — Thông báo](#luồng-i--thông-báo)
- [Luồng J — Chat riêng với bot & avatar nhóm](#luồng-j--chat-riêng-với-bot--avatar-nhóm)
- [6. Chạy thử trọn vòng bằng một script](#6-chạy-thử-trọn-vòng-bằng-một-script)
- [7. Bảng tra lỗi](#7-bảng-tra-lỗi)
- [8. Giới hạn khi chạy ở local](#8-giới-hạn-khi-chạy-ở-local)

---

## 1. Ba bên, ai sở hữu cái gì

```
          ┌──────────────┐
          │   FE (web)   │  trình duyệt
          └──┬────────┬──┘
             │        │
   access    │        │  access token của CMS
   token     │        │  (CÙNG một token cho cả hai)
             ▼        ▼
    ┌────────────┐   ┌──────────────────────────────┐
    │    CMS     │   │   CHAT (service này)         │
    │ (Node/BE)  │   │   Go · Fiber · PostgreSQL    │
    └─────┬──────┘   └────┬──────────────────┬──────┘
          │               │                  │
          │  ① Chat gọi CMS: webhook ký HMAC │
          │ ◄─────────────┘                  │
          │                                  │
          │  ② CMS gọi Chat: Bot API         │
          └─────────────────────────────────►┘
             (X-Chat-Service-Key)
```

**Chat không có trang đăng nhập.** FE lấy token từ CMS (`POST /api/client/v1/workspace/auth/login`),
rồi dùng **chính token đó** gọi Chat. Chat xác thực bằng JWKS của CMS (RS256).

| | Nguồn sự thật |
|---|---|
| **CMS** | tài khoản · nhóm + thành viên · **đơn/task** · nội dung card · tổ (team) |
| **Chat** | `actorId` · tin nhắn · `cardVersion` · trạng thái đã đọc · presence · thông báo |

Nghĩa là: **Chat không tạo đơn, không hiểu nghiệp vụ đơn.** Chat lưu card và chuyển event.

Ba định danh của cùng một con người:

| Tên | Của ai | Dạng | Ví dụ |
|---|---|---|---|
| `cmsUserId` | CMS | số | `6` |
| `openId` | CMS | `ou_…` hoặc `local:<uuid>` | `local:c2749c5b-…` |
| `actorId` | **Chat** | `u_<cmsUserId>` | `u_6` |

Và của cùng một nhóm:

| Tên | Của ai | Dạng | Ví dụ |
|---|---|---|---|
| `chatId` / `groupId` | CMS | `wg_<hex>` | `wg_00f2d7a7021bdf59` |
| id nội bộ | Chat | UUID | `acf3c341-80c7-…` |

**Mọi endpoint của Chat nhận cả hai dạng** — trừ `PUT /bot/v1/groups/:id` (bắt buộc `wg_…`, vì
đó là chatId mà CMS muốn tạo).

---

## 2. Ba bề mặt API

| Bề mặt | Ai gọi | Xác thực | Envelope | Tên field |
|---|---|---|---|---|
| `/api/client/v1/chat/*` (11 endpoint) | **FE** | `Authorization: Bearer <access token CMS>` | `{statusCode, code, message, element}` | **camelCase** |
| `/bot/v1/*` (14 endpoint) | **CMS** | `X-Chat-Service-Key` | không có — JSON phẳng | camelCase |
| `/api/v1/*` | Postman, CLI, service nội bộ | JWT | `{data, meta}` | snake_case |

Realtime cũng có hai đường, cùng một hub:

| | Dùng cho | Tên field |
|---|---|---|
| `ws://host/api/client/v1/chat/ws?token=` | **FE** | camelCase, **trùng tên với REST** |
| `ws://host/ws?token=` | công cụ nội bộ (`make ws`, Postman) | snake_case, nguyên struct domain |

---

## 3. Chuẩn bị để chạy thử

Dán cả khối này vào terminal:

```bash
cd chat-grpc        # thư mục gốc của repo

export BASE=http://localhost:8080
export KEY=$(grep '^CHAT_SERVICE_KEY=' .env | cut -d= -f2 | tr -d ' ')
export TOK=$(make -s token-cms CMS_USER=6)     # token dev đóng vai user CMS id 6 = "tester"
export GROUP=wg_00f2d7a7021bdf59               # nhóm "TEAM TEST"
export TEAM=9                                  # tổ DEV-TEST của nhóm đó
export ACTOR=u_6                               # actorId của tester

echo "BASE=$BASE  GROUP=$GROUP  TEAM=$TEAM  ACTOR=$ACTOR"
echo "TOK dai ${#TOK} ky tu, KEY dai ${#KEY} ky tu"
```

Kiểm nhanh ba thứ sống:

```bash
curl -s $BASE/healthz
curl -s $BASE/api/client/v1/chat/auth/me -H "Authorization: Bearer $TOK" | head -c 120; echo
curl -s $BASE/bot/v1/groups -H "X-Chat-Service-Key: $KEY" | head -c 120; echo
```

Nhóm và tổ của **workspace của bạn** có thể khác. Lấy đúng giá trị:

```bash
# Nhóm nào + tổ nào của nhóm đó
curl -s $BASE/api/client/v1/chat/bootstrap -H "Authorization: Bearer $TOK" | python3 -c '
import sys, json
for g in json.load(sys.stdin)["element"]["groups"]:
    teams = [(t["id"], t["code"]) for t in (g.get("cms") or {}).get("teams", [])]
    print((g["chatId"] or g["id"]).ljust(22), g["name"].ljust(16), "teams =", teams)'
```

```
wg_00f2d7a7021bdf59    TEAM TEST        teams = [(9, 'DEV-TEST')]
wg_91aeddfc3a825552    TEAM TEST 222    teams = [(9, 'DEV-TEST')]
```

---

## 4. Danh mục đầy đủ các loại "type"

Có **bốn** danh mục khác nhau, hay bị lẫn vào nhau.

### 4.1 Frame FE **gửi lên** WebSocket — 7 loại

| `type` | Payload | Có `ack`? |
|---|---|---|
| `ping` | — | `pong` |
| `presence.list` | `conversationId` (tuỳ chọn) | có — danh sách ai đang online |
| `message.send` | `conversationId`, `text`, `type`, `threadRootId`, `replyToId`, `attachments[]`, `clientMsgId` | có |
| `message.read` | `conversationId`, `messageId` | có (`unread_count`) |
| `message.delete` | `messageId` | có (`deleted`) |
| `typing` | `conversationId`, `typing` | **không** |
| `template.send` | `conversationId`, `bot_slug`, `template_code`, `payload`, `clientMsgId` | có |

Mỗi frame là một JSON object, `id` do client tự đặt và được trả lại trong `ref`:

```json
{"id":"c1","type":"message.send","payload":{"conversationId":"wg_00f2d7a7021bdf59","text":"hello","clientMsgId":"u1"}}
```

Khoá payload nhận **cả camelCase lẫn snake_case** (`conversationId` = `conversation_id`,
`text` = `content`), camelCase thắng khi cùng xuất hiện.

#### `presence.list` — ai đang online

Event `presence` chỉ bắn **khi có người đổi trạng thái**. Lúc mới mở tab thì chưa có event nào,
nên cần hỏi một lần để lấy **ảnh chụp hiện tại**:

```json
{"id":"p1","type":"presence.list"}
```

Không truyền gì = mọi người **dùng chung ít nhất một nhóm** với bạn. Giới hạn trong một nhóm:

```json
{"id":"p2","type":"presence.list","payload":{"groupId":"wg_00f2d7a7021bdf59"}}
```

Trả về (bề mặt FE, camelCase):

```json
{"type":"ack","ref":"p1","payload":{
  "count": 6,
  "items": [
    {"userId":"6a63be69-…","actorId":"u_6","name":"tester",
     "presence":"online","online":true,"lastSeenAt":"2026-11-20T04:42:37Z"},
    {"userId":"1b007c26-…","actorId":"u_7","name":"tester1",
     "presence":"offline","online":false}
  ]}}
```

| Field | Nghĩa |
|---|---|
| `presence` | `online` · `offline` · `off_duty` (nghỉ theo lịch, CMS đặt) |
| `online` | `true` chỉ khi `presence == "online"` — tiện cho FE khỏi so chuỗi |
| `reason` | chỉ có khi `off_duty` — lý do CMS gửi kèm (vd "Lịch OFF ca chiều") |
| `lastSeenAt` | lần cuối đổi trạng thái; vắng mặt nếu chưa bao giờ kết nối |

Lỗi:

| Tình huống | Trả về |
|---|---|
| `groupId` không tồn tại | `error` `conversation_not_found` |
| không phải thành viên nhóm đó | `error` `not_conversation_member` |

Đọc từ bảng `users`, **không** từ map kết nối của instance — chạy nhiều pod vẫn đúng.

#### `online` được tính thế nào

Cột `users.presence` được ghi khi WebSocket mở/đóng. Nếu process bị kill (restart, crash, OOM)
thì hook đóng kết nối **không chạy** và cột đó mắc kẹt ở `online` mãi — FE thấy người đã tắt máy
vẫn sáng đèn.

Chống bằng heartbeat + hết hạn:

```
Mỗi instance, mỗi PRESENCE_HEARTBEAT (30s):
    UPDATE users SET last_seen_at = now()
      WHERE id IN (<người đang mở WS trên instance NÀY>) AND presence = 'online'

Lúc đọc (mọi bề mặt):
    online  =  presence == 'online'  AND  last_seen_at > now() - PRESENCE_TTL (90s)
```

Instance nào giữ kết nối thì instance đó làm tươi — **không pod nào cần biết pod khác**, nên đúng
cả khi scale ngang. Process chết thì `last_seen_at` ngừng mới và sau 90 giây mọi bề mặt tự trả
`offline`.

| Trạng thái trong DB | `last_seen_at` | API trả |
|---|---|---|
| `online` | 10s trước | `online` |
| `online` | 5 phút trước | **`offline`** ← tự lành |
| `online` | chưa bao giờ | **`offline`** |
| `off_duty` | hôm qua | `off_duty` — **không hết hạn**, do CMS đặt theo lịch nghỉ |
| `offline` | bất kỳ | `offline` |

Hai biến trong `.env`:

```bash
PRESENCE_HEARTBEAT=30s   # chu kỳ làm tươi
PRESENCE_TTL=90s         # online cũ hơn ngưỡng này = offline. Nên >= 3x heartbeat
```

`PRESENCE_TTL=0` = tắt, luôn tin cột `presence` (hành vi cũ). Đặt `TTL < 2x HEARTBEAT` thì
`config.WarnProd()` sẽ cảnh báo: mất một nhịp heartbeat là người đang online bị báo offline oan.

Một chỗ dùng chung: `User.LivePresence()` trong `internal/domain/presence.go`. Mọi bề mặt
(`bootstrap`, `presence.list`, `/bot/v1/users/:id`, `/bot/v1/groups/:id/members`, gRPC) đều gọi
hàm đó — không chỗ nào đọc thẳng `u.Presence` nữa.

Trên `/ws` (bề mặt nội bộ) khoá là snake_case: `user_id`, `actor_id`, `display_name`,
`last_seen_at`.

Hai nguồn khác cũng có presence, dùng tuỳ lúc:

| Cần gì | Dùng |
|---|---|
| ảnh chụp lúc mở app | `bootstrap` → `groups[].members[].presence` (có sẵn, khỏi gọi thêm) |
| ảnh chụp lúc bất kỳ | **`presence.list`** |
| cập nhật khi có người đổi | event `presence` |

### 4.2 Frame server **gửi xuống** — 4 loại riêng + 8 event

Bốn frame trả lời trực tiếp:

| `type` | Khi nào |
|---|---|
| `ready` | ngay sau khi kết nối — **nhìn frame này trước tiên** để biết tab đang là ai |
| `ack` | frame vừa gửi đã xử lý xong (có `ref`) |
| `error` | frame bị từ chối (có `ref` + `code`) |
| `pong` | trả lời `ping` |

Tám event phát theo nhóm/người (envelope `{type, eventId, conversationId, payload, ts}`):

| `type` | Khi nào | Payload chính |
|---|---|---|
| `message` | tin mới **hoặc** tin/card được cập nhật | cả tin nhắn (FE **upsert theo `id`**). Có `threadRootId` → tin trong thread: không chèn vào khung chính, tăng `threadCount` tin gốc ([Luồng D](#thread-gom-vào-một-tin-gốc)) |
| `message.deleted` | ai đó xoá tin | `conversationId`, `messageId` |
| `read` | ai đó đánh dấu đã đọc | `conversationId`, `userId`, `messageId`, `seenAt` |
| `typing` | ai đó đang nhập | `conversationId`, `userId`, `typing` |
| `presence` | người chung nhóm đổi trạng thái | `actorId`, `presence`, `online`, `reason` |
| `notification` | có thông báo cho mình | `id`, `text`, `read`, `createdAt`, `messageId`, `conversationId`, `groupId`, `threadRootId` (chỉ có khi tin nằm trong thread) |
| `user` | hồ sơ người dùng đổi | hồ sơ rút gọn |
| `reload` | nhóm đổi → tải lại danh sách | `reason`, `conversationId`, `chatId` |

Card đổi **không** có type riêng — nó đến bằng `message` với `cardVersion` mới.

Ba event nữa chỉ có khi bật module task nội bộ (`TASK_ENGINE_ENABLED=true`, mặc định tắt):
`bot.submission.stored`, `task.created`, `task.updated`.

### 4.3 Event Chat **gửi sang CMS** (webhook, ký HMAC) — 4 loại

| `type` | Khi nào | Đồng bộ? |
|---|---|---|
| `form.submit` | FE gửi form (`POST /forms/:formId/submit`) | **có** — chờ CMS trả lời, UI cần kết quả |
| `card.action` | FE bấm nút trên card | **có** |
| `message.created` | có tin nhắn mới trong nhóm | không — qua **outbox**, có retry backoff |
| `user.updated` | hồ sơ người dùng đổi | không — qua outbox |

Khung chung mọi event:

```json
{"schema":"abc.event/v1","eventId":"evt_<uuid>","type":"form.submit",
 "createdAt":"2026-11-20T03:00:00Z", "botId":"b_1", "...": "tuỳ loại"}
```

Chữ ký: `X-Abc-Signature: hex(HMAC_SHA256(CHAT_SERVICE_KEY, "<timestamp>.<rawBody>"))`,
kèm `X-Abc-Timestamp`. Ký trên **đúng bytes đã gửi** — serialize lại rồi ký là sai.

### 4.4 `action` trên card — **do CMS định nghĩa**, Chat không có danh sách

Chat chỉ đọc khoá `action` trong card để biết "nút này tên gì" và kiểm `visibleTo`. Tên action
(`accept_task`, `submit_work`, `pause`, …) là hợp đồng **CMS ↔ FE**.

```json
{"type":"actions","items":[
  {"action":"accept_task","label":"Nhận task","placement":"primary","visibleTo":["u_6"],
   "confirm":{"title":"Nhận phần 2/3?"}}]}
```

Đây là cấu trúc CMS đang gửi. Chat **không phụ thuộc hình dạng**: nó duyệt sâu toàn bộ card,
object nào có khoá `action` là một nút — nằm trong `actions.items[]`, hay kiểu cũ
`{"type":"button","text":…,"action":…}`, đều được.

**Lọc theo `visibleTo` chỉ bỏ đúng một thứ**: object có `visibleTo` không chứa người xem. Mọi
phần tử / khoá Chat không nhận ra (`parts`, `links`, `form.rows`, trường form kiểu `parts`…) và
giá trị `null` được giữ **nguyên**. `parts.items[].assignee.actor` là dữ liệu hiển thị, không
phải `visibleTo` — Chat không lọc theo nó.

`values` của action là **JSON tuỳ ý** (lồng nhau được, vd `split_task` gửi `values.parts` là
mảng object) — Chat chuyển nguyên sang CMS. FE bỏ hẳn `values` thì CMS nhận `{}`.

Action CMS đang dùng (cập nhật 20/11/2026) — Chat không cần biết danh sách này, ghi lại để tra:

| `action` | Ai thấy | `values` |
|---|---|---|
| `reassign_task` | admin, trợ lý | `assigneeOpenId`, `note` |
| `adjust_status` | admin, trợ lý | `status`, `note` (bắt buộc khi `held`) |
| `split_task` | admin, trợ lý | `parts[]` |
| `sync_delivery_drive` | người nhận, admin, trợ lý | không có |
| `accept_task` trên card phần 2..N | người nhận phần đó | `{}` (nút confirm, không có form) |

> ⚠️ Khoá là **`action`**, KHÔNG phải `actionId`. Sai tên thì card vẫn tạo được nhưng bấm nút trả
> `403 action_not_allowed`.

FE không tự đoán: mỗi tin nhắn có card đều kèm `actions[]` = đúng những action người đang xem
được bấm.

---

## 5. Khi nào CMS cần gọi sang Chat

Đây là câu hỏi hay gặp nhất. CMS **không** cần gọi Chat liên tục — chỉ 6 thời điểm:

| Bên CMS vừa xảy ra | Gọi | Bắt buộc? |
|---|---|---|
| Tạo đơn mới | `POST /bot/v1/messages` (kèm `card`) | **có** — không gọi thì nhóm không thấy đơn |
| Đơn đổi trạng thái | `PATCH /bot/v1/messages/:id` (gộp card + thread + notify) | **có** |
| Giao việc cho ai | nằm trong `notify` của `PATCH` ở trên, hoặc `POST /bot/v1/notifications` | nên |
| Chia task thành N phần | `POST /bot/v1/messages/:cardGốc/thread` kèm `card` × (N−1) + `PATCH` card gốc — xem [Luồng G](#chia-task-card-nằm-trong-thread) | **có** |
| Nhóm / thành viên đổi | `PUT /bot/v1/groups/:chatId` | không bắt buộc — Chat tự đồng bộ mỗi 60s, gọi để **nhanh hơn** |
| Cần `actorId` của người chưa vào Chat | `POST /bot/v1/users/resolve` | khi cần điền `visibleTo` |
| Lịch OFF / hết ca | `PUT /bot/v1/presence` | tuỳ chọn |
| Tag người làm / TL / Trợ lý trên card | `mentions: [actorId…]` trong `POST /bot/v1/messages` (hoặc `/thread`) | nên — để họ thấy dấu "@" |

**Không cần gọi** khi: người dùng gửi tin nhắn, đánh dấu đã đọc, đang nhập — Chat tự xử lý và
tự gửi `message.created` sang CMS.

### Nhắc tên (@) — ai làm gì

```
Tạo đơn:  CMS ──POST /bot/v1/messages {card, mentions:[người làm, TL, Trợ lý]}──► Chat
            Chat: lưu mentions → "@ bạn" (mentionCount) — KHÔNG gửi thông báo (CMS đã gửi)
          CMS ──POST /bot/v1/notifications──► người làm, TL, Trợ lý

Nhắn @:   FE ──POST /chat/groups/:id/messages {text, mentions:["u_55"], mentionAll}──► Chat
            Chat: @all chỉ admin/trợ lý (403) · lọc thành viên · lưu · realtime `message`
                  · mỗi người 1 `notification` "X nhắc đến bạn trong …"
          Chat ──message.created {message:{…, mentions, mentionAll}}──► CMS (outbox)

Đổi tên:  Chat ──GET /workspace/users?updatedSince=…──► CMS  (mỗi CMS_USER_SYNC_INTERVAL)
            Chat: cập nhật actor → realtime `user` → chip @ hiện tên mới
```

Hai điều CMS **phải lưu**:

1. **`messageId`** mà `POST /bot/v1/messages` trả về, ghép với mã đơn. Không có nó thì không
   `PATCH` được card về sau.
2. **`cardVersion`** hiện tại của mỗi card, để lần `PATCH` sau gửi số **lớn hơn**.

---

## Luồng A — Mở app

**Ai gọi**: FE. **Khi nào**: mở trang, hoặc sau khi refresh token.

```
FE ──1──► CMS   POST /api/client/v1/workspace/auth/login {identifier, password}
   ◄─────       {accessToken, refreshToken, user}

FE ──2──► CHAT  GET /api/client/v1/chat/bootstrap        Bearer <accessToken>
   ◄─────       {me, groups[], chats[], chatsPageInfo, notifications[], unreadNotifications}
                 (lần đầu Chat tự: kiểm token qua JWKS → tạo actor
                  → GET /workspace/users?ids= lấy hồ sơ từ CMS)

FE ──3──► CHAT  ws://…/api/client/v1/chat/ws?token=<accessToken>
   ◄─────       {"type":"ready","payload":{userId, actorId, name, role, connId, serverNow}}
```

Chat **không có** bước 1 — không login, không phát token.

```bash
curl -s "$BASE/api/client/v1/chat/bootstrap" -H "Authorization: Bearer $TOK" | python3 -m json.tool | head -40
```

Trả về gì:

| Khoá | Dùng để |
|---|---|
| `me` | `actorId`, `name`, `avatarUrl`, `role`, `profile.webTeams` |
| `groups[]` | vẽ sidebar: `id`, `chatId`, `name`, `unreadCount`, `members[]`, `cms.teams[]` |
| `pinnedChats[]` | nhóm **mình đã ghim**, ghim gần nhất trên cùng — vẽ trên đầu thanh bên. Ghim / bỏ ghim: `POST /api/client/v1/chat/groups/:id/pin` `{"pinned": true\|false}` (riêng từng người) |
| `chats[]` + `chatsPageInfo` | **vẽ thanh bên** kiểu Lark: nhóm + thread trộn chung, 20 dòng đầu (không gồm nhóm đã ghim). Cuộn xuống: `bootstrap?chatCursor=<nextCursor>` ([Luồng D](#thanh-bên-kiểu-lark--chats-trong-bootstrap)) |
| `notifications[]` | 30 thông báo mới nhất |
| `unreadNotifications` | số cho badge |

**Một request là đủ để vẽ xong sidebar** — đừng gọi từng nhóm.

Token hết hạn (401 ở bất kỳ request nào) → FE gọi CMS `POST /workspace/auth/refresh`, rồi
**mở lại WebSocket** bằng token mới.

---

## Luồng B — Mở một nhóm, đọc tin

**Ai gọi**: FE.

```bash
# 1. Đọc 30 tin mới nhất
curl -s "$BASE/api/client/v1/chat/groups/$GROUP/messages?limit=30" \
  -H "Authorization: Bearer $TOK" | python3 -c '
import sys, json
el = json.load(sys.stdin)["element"]
for m in el["messages"][:5]:
    print(m["senderType"].ljust(5), (m.get("text") or "")[:40].ljust(42),
          "card =", bool(m.get("card")), "actions =", m.get("actions"))
print("nextCursor =", el["pageInfo"]["nextCursor"][:24], "| hasMore =", el["pageInfo"]["hasMore"])'
```

| Tham số | Nghĩa |
|---|---|
| `limit` | 1–100, mặc định 30 |
| `cursor` | `pageInfo.nextCursor` của trang trước — **cuộn lên** lấy tin cũ hơn |
| `threadRootId` | mở một thread: tin gốc + mọi tin trả lời trong thread đó |
| `includeThread` | `true` = lấy cả tin trong thread lẫn vào lịch sử (hành vi cũ, chỉ để debug) |

**Lịch sử nhóm chỉ có tin gốc.** Tin trả lời trong thread (có `threadRootId`) **không** nằm
trong danh sách này — chúng gom vào tin gốc qua `threadCount` ("N trả lời"), bấm vào mới tải
(Luồng D). Card đơn, tin hệ thống "đã gửi yêu cầu" là tin gốc nên vẫn hiện; dòng diễn biến
CMS ghi vào thread của card ("Đã giao…", tin Google Drive, "SKY · Nhận task") thì chỉ thấy
trong thread.

```bash
# 2. Đánh dấu đã đọc tới tin mới nhất
MSG=$(curl -s "$BASE/api/client/v1/chat/groups/$GROUP/messages?limit=1" \
  -H "Authorization: Bearer $TOK" | python3 -c 'import sys,json;print(json.load(sys.stdin)["element"]["messages"][0]["id"])')

curl -s -X POST "$BASE/api/client/v1/chat/groups/$GROUP/read" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"messageId\":\"$MSG\"}"; echo
```

Người khác trong nhóm nhận event `read` → cập nhật dấu đã xem.

**Phân trang là cursor, không phải offset.** Timeline chat chèn tin liên tục, offset sẽ lệch.

---

## Luồng C — Gửi tin nhắn

**Ai gọi**: FE. Hai đường cho cùng một việc:

```
WebSocket (ưu tiên):  {"id":"c1","type":"message.send","payload":{...}}  → ack
REST (khi WS đứt):    POST /api/client/v1/chat/groups/{id}/messages      → element.message
```

```bash
curl -s -X POST "$BASE/api/client/v1/chat/groups/$GROUP/messages" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"text":"Xin chào cả nhóm","clientMsgId":"'$(uuidgen)'"}' \
  | python3 -c 'import sys,json;d=json.load(sys.stdin)["element"];print("id =", d["message"]["id"], "| duplicated =", d["duplicated"])'
```

| Tham số | Bắt buộc | Nghĩa |
|---|---|---|
| `text` | có (hoặc `attachments`) | nội dung |
| `clientMsgId` | **nên** | UUID do FE sinh — **chống gửi trùng** |
| `replyToId` | không | trả lời một tin |
| `threadRootId` | không | gửi thẳng vào thread |
| `attachments[]` | không | `{kind, url, name, mime_type, size_bytes, width, height}` |
| `metadata` | không | JSON tự do, Chat không đọc |

**`clientMsgId` quan trọng**: gửi lại cùng giá trị → server trả tin cũ với `duplicated: true` và
**không phát cho ai**. Đó là cách retry an toàn khi mạng chập chờn — nhưng khi test tay thì mỗi
lần phải đổi giá trị, không thì tưởng server hỏng.

Chat **không lưu file**, chỉ lưu URL. Upload là service khác.

---

## Luồng D — Trả lời & thread

```bash
# Trả lời tin $MSG → tạo thread
RE=$(curl -s -X POST "$BASE/api/client/v1/chat/groups/$GROUP/messages" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"text\":\"Trả lời trong thread\",\"replyToId\":\"$MSG\",\"clientMsgId\":\"$(uuidgen)\"}" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["element"]["message"]["id"])')
echo "tin tra loi = $RE"

# Mở cả thread của tin gốc
curl -s "$BASE/api/client/v1/chat/messages/$MSG/replies?mode=thread&limit=50" \
  -H "Authorization: Bearer $TOK" | python3 -c '
import sys,json
el = json.load(sys.stdin)["element"]
print("root =", (el["root"].get("text") or "")[:40])
print("so phan hoi =", len(el["replies"]), "| replyCount =", el.get("replyCount"), "| threadCount =", el.get("threadCount"))'
```

| `mode` | Trả về |
|---|---|
| `direct` | chỉ tin trả lời **trực tiếp** tin đó (`replyToId`) |
| `thread` | **cả thread** mà tin đó mở (`threadRootId`) |

Tin gốc giữ nguyên, chỉ `replyCount` / `threadCount` tăng → FE vẽ badge.

### Thread gom vào một tin gốc

```
Khung chat chính (GET …/groups/{id}/messages)       Panel thread (bấm vào tin gốc)
┌──────────────────────────────────────┐            ┌──────────────────────────────────────┐
│ [card WEB-…-0001]   💬 3 trả lời  ───┼── bấm ───► │ GET …/groups/{id}/messages            │
│ SKY · Đã gửi yêu cầu WEB-…-0002      │            │       ?threadRootId={id tin gốc}      │
│ An: Xin chào cả nhóm                 │            │ hoặc GET …/messages/{id}/replies      │
└──────────────────────────────────────┘            │       ?mode=thread                    │
  KHÔNG có tin trả lời trong thread                  │ → tin gốc + các tin trả lời cũ → mới  │
                                                     └──────────────────────────────────────┘
```

**Realtime**: tin trả lời trong thread vẫn tới qua event `message`, payload có `threadRootId`.

| Event `message` | FE làm |
|---|---|
| **không** có `threadRootId` | upsert vào khung chat chính theo `id` (như cũ) |
| **có** `threadRootId` | **không** chèn vào khung chính — tăng `threadCount` của tin gốc; nếu panel thread đó đang mở thì upsert vào panel |

### Thanh bên kiểu Lark — `chats` trong `bootstrap`

Thanh bên là **một danh sách trộn nhóm và thread**, hoạt động gần nhất trước (giống Lark). **Không
có API riêng**: trang đầu nằm sẵn trong `GET /api/client/v1/chat/bootstrap`, cuộn xuống thì lazy load:

```
GET /bootstrap                              → me, groups, chats (trang đầu), chatsPageInfo, notifications…
GET /bootstrap?chatCursor=<nextCursor>      → CHỈ {chats, chatsPageInfo} (không tải lại nhóm / thông báo)
    &chatLimit=20                             mặc định 20, tối đa 100
```

`chatsPageInfo.hasMore = false` → hết, ngừng lazy load. `groups` vẫn trả đủ (thành viên, `cms.teams`)
để tra cứu — **vẽ thanh bên bằng `chats`**, không phải `groups`.

```json
{ "chats": [
    { "type": "thread",
      "id": "3a0ea6e2-…",                              // = threadRootId
      "title": "WEB-20261123-0001 · Banner Tết",        // card: "code · title"; tin thường: dòng đầu
      "preview": "Designer Bot: 📁 **Google Drive** …",  // "Người gửi: nội dung" của tin mới nhất
      "lastActivityAt": "2026-11-23T08:53:55Z",
      "group": { "id": "4075d650-…", "chatId": "wg_91aeddfc3a825552", "name": "TEAM TEST 222", "avatarUrl": "" },
      "root": { …tin gốc, card đã lọc visibleTo… },
      "lastMessage": { …tin mới nhất trong thread… },
      "threadCount": 2,          // mọi tin trong thread
      "replyCount": 0 },         // chỉ tin người dùng → 0 = "Chưa có phản hồi"
    { "type": "group",
      "id": "4075d650-…",
      "title": "TEAM TEST 222",
      "preview": "Designer Bot: WEB-20261123-0001 · Banner Tết",
      "lastActivityAt": "2026-11-23T08:53:55Z",
      "group": { …chính nhóm… },
      "lastMessage": { …tin GỐC mới nhất của nhóm… },
      "unreadCount": 4 }
  ],
  "chatsPageInfo": { "hasMore": true, "nextCursor": "…" } }
```

| `type` | Là gì | `lastActivityAt` | Bấm vào |
|---|---|---|---|
| `group` | một nhóm mình là thành viên | tin **gốc** mới nhất (tin trong thread **không** đẩy nhóm lên) | `GET /groups/{id}/messages` |
| `thread` | tin gốc có ≥ 1 tin trả lời, hoặc là card (card đơn luôn mở thread) | tin mới nhất trong thread | `GET /groups/{group.id}/messages?threadRootId={id}` |

**Realtime cho thanh bên** (không có event riêng — dùng event `message` sẵn có):

| Event `message` | Thanh bên |
|---|---|
| có `threadRootId` | dòng `thread` có `id = threadRootId` lên đầu, cập nhật `preview`/`lastMessage`, `threadCount` +1 (`replyCount` +1 nếu `senderType = "user"`). Chưa có trong danh sách → gọi lại `bootstrap` |
| không có `threadRootId`, có `card` | dòng `group` lên đầu **và** thêm dòng `thread` mới (card mở thread) |
| không có `threadRootId` | dòng `group` có `id = conversationId` lên đầu, cập nhật `preview`, `unreadCount` +1 nếu không phải tin của mình |

Chỉ có **mã đơn** (từ thông báo, link) mà chưa có id tin gốc: `GET /api/client/v1/chat/cards/{code}`
trả card + `links` (Drive) + thread đã tách `activity` (bot/hệ thống) / `discussion` (người dùng) —
xem [G1](#g1-link-google-drive-của-đơn).

---

## Luồng E — Tạo đơn (vòng tròn đầy đủ)

Đây là luồng dài nhất và hay hiểu sai nhất.

```
FE  ──1──► CMS   GET /workspace/teams… (chọn tổ, gợi ý người nhận)      [API của CMS]

FE  ──2──► CHAT  POST /api/client/v1/chat/forms/create_task/submit
                   {values:{kind,teamId,normItemId,title,brief,quantity,size,
                            urgencyCode,useAI,assigneeOpenId},
                    context:{groupId, threadRootId?}}

           CHAT kiểm: token hợp lệ? người gọi ∈ nhóm?  → không thì 403
           CHAT KHÔNG đọc values — chỉ đóng gói rồi ký HMAC

           CHAT ──3──► CMS   webhook form.submit  (ĐỒNG BỘ, chờ tối đa CMS_WEBHOOK_TIMEOUT)
                 CMS tạo đơn WEB-YYYYMMDD-NNNN, chọn người nhận / xếp hàng
                 CMS ──4──► CHAT  POST /bot/v1/users/resolve  → actorId (nếu cần)
                 CMS ──5──► CHAT  POST /bot/v1/messages {groupId, bot, card}
                                   → {messageId, cardVersion:1}
                              CHAT ══► realtime "message" (card lọc visibleTo cho TỪNG người)
                 CMS ──6──► CHAT  PATCH /bot/v1/messages/{id} {card, cardVersion, thread, notify}
                              (hoặc gọi riêng /thread và /notifications)
           CHAT ◄──7── CMS   200 {ok, toast, closeForm}

           ok=true → CHAT ghi 1 tin system vào nhóm (thread nếu có threadRootId):
               "SKY · Đã gửi yêu cầu WEB-…"  metadata.kind="form_submitted" {form_id, values, toast}
               ══► realtime "form.submitted"  (KHÔNG phải "message" — FE có thể bỏ qua)

FE  ◄──8── CHAT  {statusCode:200, element:{ok, toast, closeForm}}
```

**FE chỉ gọi một request** (bước 2). Bước 3–7 là Chat ↔ CMS.

```bash
curl -s -X POST "$BASE/api/client/v1/chat/forms/create_task/submit" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"values\":{\"kind\":\"task\",\"teamId\":\"$TEAM\",\"normItemId\":\"74\",
                 \"title\":\"Banner Tết 2027\",\"brief\":\"Tông đỏ vàng, có logo\",
                 \"quantity\":\"2\",\"size\":\"1200x628\",\"urgencyCode\":\"normal\",\"useAI\":false},
       \"context\":{\"groupId\":\"$GROUP\"}}" | python3 -m json.tool
```

Response:

```json
{"statusCode": 200, "code": "SUCCESS", "message": "Success",
 "element": {"ok": true,
             "toast": {"type": "success", "text": "Đã gửi yêu cầu WEB-20261120-0012"},
             "closeForm": true}}
```

### Ba tầng "thành công" khác nhau — đọc đúng tầng

| Field | Nghĩa |
|---|---|
| `statusCode: 200` | **Chat** chuyển sang CMS được và nhận được trả lời |
| `ok: true` | **CMS** nhận và xử lý được request |
| `toast.type` + `closeForm` | **kết quả nghiệp vụ** ← cái FE phải đọc |

```js
if (el.toast) toast(el.toast.text, el.toast.type);
if (el.closeForm) closeForm();       // false = chưa xong, giữ form để sửa
```

Gửi tổ không thuộc nhóm → vẫn `200` + `ok: true`, nhưng:

```json
{"ok": true, "toast": {"type": "error", "text": "Chọn tổ thực hiện thuộc nhóm này"},
 "closeForm": false}
```

`teamId` **phải lấy từ** `bootstrap` → `groups[].cms.teams[].id`, đừng hardcode. Chat không đọc
`values` nên không chặn sớm được.

### Card đơn KHÔNG đến từ response này

Nó xuất hiện ở **bước 5**, khi CMS gọi `POST /bot/v1/messages`. FE chỉ cần nghe event `message`
và upsert theo `id`.

`values` có những trường gì là hợp đồng **CMS ↔ FE** (`CHAT_SERVICE_INTEGRATION.md` mục 6.2):
`kind`, `title`, `brief`, `teamId`, `useAI`, `assigneeOpenId`, `assigneeName`. Thêm trường mới
**không phải sửa Chat**.

---

## Luồng F — Bấm nút trên card

```
FE ──1──► CHAT  POST /api/client/v1/chat/messages/{messageId}/actions
                  {action:"accept_task", values:{normItemId, quantity}}

          CHAT kiểm: action có trong card? người bấm ∈ visibleTo?
               không → 403 ACTION_NOT_ALLOWED  (dừng, KHÔNG gọi CMS)

          CHAT ──2──► CMS  webhook card.action
                           {messageId, action, values, actor, group, cardVersion}
                CMS cập nhật đơn (khoá lạc quan theo cardVersion)
          CHAT ◄──3── CMS  200 {ok, toast, closeForm, card, cardVersion:2}

          ok=true → CHAT ghi 1 tin system vào THREAD của card (card con → thread gốc):
               "SKY · Nhận việc"  metadata.kind="card_action" {action, values, card_message_id, card_version}
          CHAT thay card nếu cardVersion LỚN HƠN bản đang lưu
               ══► realtime "message" (mỗi người một bản card đã lọc)

FE ◄──4── CHAT  {ok, toast, closeForm, card (đã lọc), cardVersion}
```

```bash
# Tìm một card mình được bấm, rồi bấm đúng action đó
read CARD ACTION <<<$(curl -s "$BASE/api/client/v1/chat/groups/$GROUP/messages?limit=30" \
  -H "Authorization: Bearer $TOK" | python3 -c '
import sys,json
for m in json.load(sys.stdin)["element"]["messages"]:
    if m.get("card") and m.get("actions"):
        print(m["id"], m["actions"][0]); break
else: print("", "")')
echo "card=$CARD action=$ACTION"

curl -s -X POST "$BASE/api/client/v1/chat/messages/$CARD/actions" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"action\":\"$ACTION\",\"values\":{\"normItemId\":\"996802\",\"quantity\":2}}" | python3 -m json.tool
```

| Tham số | Nghĩa |
|---|---|
| `action` | tên action — **phải nằm trong `actions[]`** của card |
| `values` | dữ liệu form của nút (nếu nút có form), Chat chuyển nguyên sang CMS |

Lỗi nghiệp vụ: CMS vẫn trả **200** nhưng `toast.type: "error"`, `closeForm: false`, **không có
`card`** → FE chỉ hiện toast, giữ nguyên card cũ.

---

## Luồng G — CMS cập nhật đơn

**Ai gọi**: CMS. Không có FE trong luồng này — FE chỉ nhận realtime.

**Một request làm cả ba việc** (thay cho 3 request riêng):

```bash
curl -s -X PATCH "$BASE/bot/v1/messages/$CARD" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d '{
    "card": {"schema":"abc.card/v1",
             "header":{"code":"WEB-20261118-0006","title":"Banner Tết 2027",
                       "status":{"text":"Đã nộp bài","tone":"success"}},
             "elements":[{"type":"text","text":"Chờ duyệt"}]},
    "cardVersion": 9,
    "thread": {"text":"Trần Designer đã nộp bài · Chờ duyệt","bot":{"id":1,"name":"Designer Bot"}},
    "notify": [{"actorId":"'$ACTOR'","text":"Đơn WEB-20261118-0006 đã nộp bài"}]
  }' | python3 -m json.tool
```

```json
{"ok": true, "cardVersion": 9,
 "thread": {"messageId": "bba88da4-…"},
 "notify": [{"id": "154935fa-…", "actorId": "u_6", "read": false, "createdAt": 1795146120060}]}
```

| Tham số | Bắt buộc | Nghĩa |
|---|---|---|
| `card` | **có** | card mới, thay toàn bộ |
| `cardVersion` | **có** | phải **lớn hơn** bản đang lưu |
| `thread.text` | không | ghi một dòng diễn biến vào thread của card |
| `thread.bot` | không | `{id, name}` — người gửi dòng đó |
| `notify[]` | không | `{actorId, text, messageId?}` — `messageId` trống thì tự trỏ về card này |

### Chia task: card nằm trong thread

Chia đơn thành N phần: phần 1 giữ card gốc, card của phần 2..N nằm **trong thread của card gốc**.

```bash
curl -s -X POST "$BASE/bot/v1/messages/$CARD/thread" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d '{"bot":{"id":1,"name":"Designer Bot"},
       "card":{"schema":"abc.card/v1",
               "header":{"code":"WEB-20261120-0003-P2","title":"Banner Tết · Phần 2/3"},
               "elements":[{"type":"actions","items":[
                 {"action":"accept_task","label":"Nhận task","visibleTo":["u_6"]}]}]}}'
# → {"messageId":"…","cardVersion":1,"threadRootId":"<id card gốc>"}
```

| Câu hỏi | Trả lời |
|---|---|
| `/thread` nhận `card`? | **Có.** Body cần `text` **hoặc** `card` (cả hai cũng được). Không gửi `cardVersion` thì card bắt đầu từ `1`; response trả `messageId` + `cardVersion`. |
| `PATCH /bot/v1/messages/:id` với tin trong thread? | **Có**, y như card ở timeline (cùng luật `cardVersion`). Field `thread` của PATCH ghi dòng vào **thread của card gốc**, `notify[].messageId` bỏ trống thì trỏ về chính card phần. |
| FE bấm nút trên card trong thread? | `POST …/messages/:id/actions` với `:id` = tin trong thread: Chat kiểm `visibleTo` rồi gửi `card.action` với `messageId` = **id của tin trong thread**. |
| Lịch sử nhóm có hiện tin trong thread không? | **Không.** `GET …/groups/:id/messages` mặc định chỉ trả tin gốc, mỗi tin gốc có `threadCount`. Bấm vào → `?threadRootId=<id>`. Realtime: tin có `threadRootId` → tăng `threadCount` của tin gốc, không chèn vào khung chính. `?includeThread=true` để lấy cả (debug). |
| FE đọc card trong thread? | `GET …/groups/:id/messages?threadRootId=` và `GET …/messages/:id/replies?mode=thread` trả `card` + `cardVersion` + `actions[]` đã lọc theo người xem. Event realtime `message` của tin này có `threadRootId`. |
| Đường khác? | `POST /bot/v1/messages` kèm `threadRootId` + `card` cũng chạy, trả `{messageId, cardVersion}`. |
| Gọi `/thread` với `:id` là card phần? | Tin mới vẫn vào thread của card gốc — thread chỉ có một cấp. |

Chat **không có rate limit** trên `/bot/v1/*`: N lần `PATCH` liên tiếp cho N card phần chạy bình
thường. Mỗi request nên có `Idempotency-Key` riêng.

### `cardVersion` — chống ghi đè ngược

Gửi lại số **không lớn hơn**:

```bash
curl -s -X PATCH "$BASE/bot/v1/messages/$CARD" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d '{"card":{"schema":"abc.card/v1","header":{"title":"Bản cũ"},"elements":[]},
       "cardVersion":1,
       "thread":{"text":"Dòng này KHÔNG được ghi"}}' | python3 -m json.tool
```

```json
{"ok": true, "ignored": true, "cardVersion": 9}
```

`ignored: true` **không phải lỗi** — Chat đang giữ bản mới hơn. Khi đó `thread` và `notify`
**cũng bị bỏ qua**: đang gửi trạng thái cũ thì không được ghi thêm dòng diễn biến cũ.

### Lỗi của phần phụ

Card đổi xong mà `thread`/`notify` lỗi → vẫn `200`, báo theo từng phần:

```json
{"ok": true, "cardVersion": 10,
 "thread": {"error": {"code": "text_required", "message": "Thieu text"}},
 "notify": [{"id": "…", "actorId": "u_6"},
            {"error": {"code": "user_not_found", "message": "Khong tim thay nguoi dung"}}]}
```

Không trả 5xx vì CMS retry sẽ gặp `ignored: true` (cardVersion không tăng nữa) và mất luôn
thread/notify.

### G1. Link Google Drive của đơn

Chat **không gọi API nào** để lấy link Drive — CMS tạo folder xong thì tự đẩy sang, bằng đúng
hai Bot API sẵn có. Không có endpoint hay bảng riêng cho Drive: link nằm **trong card** và
**trong tin thread**, lưu bền như mọi tin khác (tải lại trang vẫn còn).

```
── ngay khi tạo đơn ──
CMS ──► CHAT  POST /bot/v1/messages {card}            → {messageId, cardVersion:1}  card CHƯA có Drive

── vài giây sau (CMS chạy nền, chỉ khi bot đã bật Drive) ──
CMS ──► CHAT  POST  /bot/v1/messages/{id}/thread {text: markdown 3 link}
                CHAT ══► realtime "message" (threadRootId = id)
CMS ──► CHAT  PATCH /bot/v1/messages/{id} {cardVersion:2, card có phần tử links}
                CHAT thay card (2 > 1) ══► realtime "message" cùng id, cardVersion 2
```

Phần tử `links` trong card (Chat lưu nguyên văn, **không lọc** vì không có `visibleTo`):

```json
{ "type": "links", "title": "Google Drive", "items": [
    { "label": "Folder đơn", "url": "https://drive.google.com/drive/folders/…" },
    { "label": "01_Tai_lieu · Tài liệu (người order up ảnh / file)", "url": "…" },
    { "label": "02_Ket_qua · Kết quả (designer up bài)", "url": "…" } ] }
```

- Từ `cardVersion: 2` trở đi, mọi card CMS gửi cho đơn này (PATCH sau, hoặc card trong
  response `card.action`) đều đã có `links`.
- **Không có khối Drive** khi: bot chưa bật / chưa cấu hình Drive, đơn tham khảo
  (`kind: reference`), hoặc CMS tạo folder lỗi. FE không cần xử lý gì đặc biệt — chỉ là card
  không có phần tử `links`.
- Đọc lại sau khi tải trang: card ở timeline nhóm; tin markdown ở
  `GET /api/client/v1/chat/messages/{id}/replies?mode=thread`.
- Chỉ có mã đơn: `GET /api/client/v1/chat/cards/{code}` trả card + `links` (khối Drive tách sẵn)
  + `activity` (bot/hệ thống) + `discussion` (người dùng) trong một request — xem
  [frontend.md](frontend.md).

---

## Luồng H — Thành viên nhóm thay đổi

**Ai gọi**: CMS (hoặc Chat tự đồng bộ mỗi 60 giây).

```bash
curl -s -X PUT "$BASE/bot/v1/groups/wg_demo_flow_md" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d '{"name":"Nhóm thử FLOW.md",
       "members":[{"openId":"local:flow-md-1","cmsUserId":9500,"name":"Người thử",
                   "sources":["team"],"teamCodes":["DESIGN"]}]}'; echo
```

```json
{"id": "wg_demo_flow_md", "memberCount": 1}
```

**Thay thế toàn bộ** danh sách: ai không có trong `members` **mất quyền xem nhóm ngay**.

| Tham số thành viên | Bắt buộc | Nghĩa |
|---|---|---|
| `openId` | **có** | Chat dùng để khớp người |
| `cmsUserId` | nên | để sinh `actorId = u_<cmsUserId>` |
| `name`, `avatarUrl` | nên | tạo/cập nhật hồ sơ nếu người đó chưa có trong Chat |
| `sources[]` | không | `team` / `assistant` / `manual` / `invite`. Có `assistant` → Chat coi là admin nhóm |
| `teamCodes[]` | không | lưu lại, trả cho FE, Chat không diễn giải |

### Chat chỉ bắn `reload` khi THẬT SỰ có thay đổi

| Thay đổi | `reason` | Gửi cho ai |
|---|---|---|
| thêm / bớt thành viên | `group.members_changed` | thành viên hiện tại **∪** người vừa bị gỡ |
| đổi tên nhóm, `avatarUrl`, `type`, `cms_info` | `group.updated` | thành viên hiện tại |
| người dùng ghim / bỏ ghim nhóm | `group.pinned` | **chỉ chính người đó** (các tab / thiết bị khác) |
| không đổi gì | *(không bắn)* | — |

Chạy lại lệnh trên lần nữa với payload y nguyên → **không ai nhận gì**. Trước đây Chat bắn vô
điều kiện mỗi 60 giây, FE tải lại danh sách nhóm liên tục dù chẳng có gì mới.

Vào nhóm bằng link mời: FE gọi CMS `POST /workspace/groups/join {token}` trước, rồi
`POST /api/client/v1/chat/groups/refresh` để Chat hỏi lại CMS ngay.

```bash
curl -s -X POST "$BASE/api/client/v1/chat/groups/refresh" -H "Authorization: Bearer $TOK" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin)["element"];print("synced =", d["synced"], "| nhom cua toi =", len(d["groups"]))'
```

`synced` là **tổng số nhóm CMS trả về**, không phải số nhóm của bạn.

---

## Luồng I — Thông báo

**Ai gọi**: CMS tạo, FE đọc.

```bash
# CMS tạo
curl -s -X POST "$BASE/bot/v1/notifications" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H 'Content-Type: application/json' \
  -d "{\"actorId\":\"$ACTOR\",\"text\":\"Bạn có task mới: WEB-20261118-0006\",\"messageId\":\"$CARD\"}" \
  | python3 -m json.tool

# FE đọc chưa đọc
curl -s "$BASE/api/client/v1/chat/notifications?unread=true&limit=20" -H "Authorization: Bearer $TOK" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin)["element"];print("tra ve", len(d["notifications"]), "| tong chua doc =", d["unread"])'

# FE đánh dấu đã đọc hết
curl -s -X POST "$BASE/api/client/v1/chat/notifications/read" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' -d '{"all":true}'; echo
```

| Tham số | Nghĩa |
|---|---|
`actorId` | ai nhận |
`text` | nội dung |
`messageId` | để FE bấm vào là nhảy tới đúng card |

Thông báo FE nhận (REST lẫn realtime, cùng tên trường):

```json
{"id":"894dcb7d-…","text":"Bạn được giao phần 2/3","read":false,"createdAt":"2026-11-20T06:19:29Z",
 "messageId":"34030601-…","conversationId":"e1a636e9-…","groupId":"wg_…",
 "threadRootId":"a8e203f3-…"}
```

`messageId` có thể là card phần nằm **trong thread**. Chat tự tra vị trí của tin: FE mở nhóm
`conversationId`; **có `threadRootId`** thì mở thread đó rồi cuộn tới `messageId`, **không có** thì
cuộn trong timeline nhóm. `groupId` (chatId của CMS) vắng mặt với nhóm tạo trong Chat.

`element.unread` là **tổng** số chưa đọc — dùng cho badge, **không đổi** theo filter `unread=true`.

---

## Luồng J — Chat riêng với bot & avatar nhóm

**Ai gọi**: CMS tạo nhóm / đổi avatar, bot CMS gửi tin, FE chỉ đọc.

```
Tạo chat riêng:  CMS ──PUT /bot/v1/groups/<chatId> {type:"dm", name, avatarUrl?, botId, members:[1 người]}──► Chat
                   (hoặc GET /workspace/groups trả nhóm có type:"dm" — Chat tự poll)
Bot nhắc việc:   CMS ──POST /bot/v1/messages {groupId:<chatId dm>, bot, text|card}──► Chat ══► realtime `message`
FE:              bootstrap.groups[].type = "dm" → ẩn ô nhập + "Tạo yêu cầu"; vẫn đọc / đánh dấu đọc / bấm nút card
Người dùng gửi:  POST …/groups/<dm>/messages hoặc forms/:id/submit → 403 BOT_DM_READ_ONLY

Đổi avatar nhóm: FE ──CMS /workspace/groups/:chatId/avatar/presign → S3 → /confirm──► CMS
                 CMS ──PUT /bot/v1/groups/:chatId {avatarUrl}──► Chat   (hoặc lần poll kế tiếp)
                 Chat ══► `reload` {reason:"group.updated"} cho thành viên → FE gọi lại bootstrap
```

`PUT` chỉ cần gửi field thay đổi: thiếu `type` / `avatarUrl` / `botId` / `teams` thì Chat **giữ
nguyên** giá trị đang có. `avatarUrl: null` = gỡ avatar.

```bash
# CMS tạo chat riêng với bot cho tester (cmsUserId 6)
curl -s -X PUT "$BASE/bot/v1/groups/dm_u6" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" -H 'Content-Type: application/json' \
  -d '{"type":"dm","name":"Designer Bot","avatarUrl":"/api/avatars/bot.png","botId":1,
       "members":[{"openId":"local:c2749c5b-…","cmsUserId":6,"name":"tester"}]}'
# → {"id":"dm_u6","type":"dm","memberCount":1}

# Người dùng thử gửi tin vào đó
curl -s -X POST "$BASE/api/client/v1/chat/groups/dm_u6/messages" -H "Authorization: Bearer $TOK" \
  -H 'Content-Type: application/json' -d '{"text":"hi bot"}'
# → 403 {"code":"BOT_DM_READ_ONLY","message":"Chat riêng với bot chỉ nhận thông báo, không gửi tin được"}
```

---

## 6. Chạy thử trọn vòng bằng một script

Cách nhanh nhất: Postman. Hai file riêng cho hai đội:

```bash
make postman-fe  TOKEN=$TOK          # 12 bước của FE
make postman-cms KEY=$KEY            # 12 bước của CMS
make postman-fe  TOKEN=$TOK STOP=3.2 # dừng ở bước 3.2 để xem kỹ
```

Hoặc bằng shell, đúng thứ tự thật:

```bash
set -e
echo "① FE mở app";            curl -s "$BASE/api/client/v1/chat/bootstrap" -H "Authorization: Bearer $TOK" >/dev/null && echo "   OK"

echo "② FE gửi tin";           MSG=$(curl -s -X POST "$BASE/api/client/v1/chat/groups/$GROUP/messages" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"text":"FLOW.md thử","clientMsgId":"'$(uuidgen)'"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["element"]["message"]["id"])'); echo "   msg=$MSG"

echo "③ FE gửi form → CMS";    curl -s -X POST "$BASE/api/client/v1/chat/forms/create_task/submit" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d "{\"values\":{\"kind\":\"task\",\"title\":\"FLOW.md\",\"teamId\":\"$TEAM\",\"useAI\":false},\"context\":{\"groupId\":\"$GROUP\"}}" \
  | python3 -c 'import sys,json;el=json.load(sys.stdin)["element"];print("   toast =", el["toast"], "| closeForm =", el["closeForm"])'

echo "④ CMS đẩy card (đóng vai CMS)"; CARD=$(curl -s -X POST "$BASE/bot/v1/messages" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" -H 'Content-Type: application/json' \
  -d "{\"groupId\":\"$GROUP\",\"bot\":{\"id\":1,\"name\":\"Designer Bot\"},
       \"card\":{\"schema\":\"abc.card/v1\",\"header\":{\"title\":\"FLOW.md · Đơn thử\"},
                 \"elements\":[{\"type\":\"button\",\"text\":\"Nhận việc\",\"action\":\"accept_task\",\"visibleTo\":[\"$ACTOR\"]}]}}" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["messageId"])'); echo "   card=$CARD"

echo "⑤ FE bấm nút";           curl -s -X POST "$BASE/api/client/v1/chat/messages/$CARD/actions" \
  -H "Authorization: Bearer $TOK" -H 'Content-Type: application/json' \
  -d '{"action":"accept_task","values":{"quantity":2}}' \
  | python3 -c 'import sys,json;el=json.load(sys.stdin)["element"];print("   toast =", el.get("toast"))'

echo "⑥ CMS cập nhật đơn (gộp 3 việc)"; curl -s -X PATCH "$BASE/bot/v1/messages/$CARD" \
  -H "X-Chat-Service-Key: $KEY" -H "Idempotency-Key: $(uuidgen)" -H 'Content-Type: application/json' \
  -d "{\"card\":{\"schema\":\"abc.card/v1\",\"header\":{\"title\":\"FLOW.md · Đã nhận\"},\"elements\":[]},
       \"cardVersion\":2,
       \"thread\":{\"text\":\"Đã nhận task\"},
       \"notify\":[{\"actorId\":\"$ACTOR\",\"text\":\"Đơn đã được nhận\"}]}" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);print("   cardVersion =", d["cardVersion"], "| thread =", d.get("thread"))'

echo "⑦ FE xem thông báo";     curl -s "$BASE/api/client/v1/chat/notifications?unread=true" -H "Authorization: Bearer $TOK" \
  | python3 -c 'import sys,json;print("   chua doc =", json.load(sys.stdin)["element"]["unread"])'
```

Bước ⑤ thường trả:

```
toast = {'type': 'error', 'text': 'Không tìm thấy task cho card này'}
```

**Đúng, không phải lỗi của Chat.** Card ở bước ④ do ta tự tạo nên **không gắn với đơn nào** trong
CMS; Chat đã chuyển `card.action` sang CMS và CMS trả lời "không có task cho card này". Muốn thấy
`success` thì card phải do CMS tạo từ một đơn thật (bước ③ trên môi trường đã thông hai chiều).

Xem bằng mắt: mở `http://localhost:8080/demo/live.html`, dán token (`echo $TOK`) — card và thông
báo hiện ra theo thời gian thực.

---

## 7. Bảng tra lỗi

### Bề mặt FE (`/api/client/v1/chat/*`)

| HTTP | `code` | Nghĩa | Làm gì |
|---|---|---|---|
| 401 | `unauthenticated` | token sai / hết hạn | gọi CMS `auth/refresh`, mở lại WebSocket |
| 403 | `NOT_CONVERSATION_MEMBER` | không thuộc nhóm | kiểm `groupId`; `bootstrap` chỉ trả nhóm của mình |
| 403 | `ACTION_NOT_ALLOWED` | action không có trong `visibleTo` | chỉ hiện nút trong `actions[]` |
| 403 | `BOT_DM_READ_ONLY` | gửi tin / form vào chat riêng với bot (`type: "dm"`) | ẩn ô nhập khi `groups[].type = "dm"` |
| 403 | `MENTION_ALL_FORBIDDEN` | `mentionAll` mà không phải admin / trợ lý | chỉ hiện `@all` khi `groups[].canMentionAll` |
| 400 | `cms_disabled` | Chat chạy không có CMS | `CMS_ENABLED=true` |
| 400 | `empty_message` | không có `text` và `attachments` | — |
| 404 | `conversation_not_found` | sai id nhóm | — |
| 500 | `cms_unreachable` | CMS không trả lời | **lỗi ở CMS hoặc mạng**, không phải Chat |
| 200 | `ok:true` + `toast.type:"error"` | **CMS từ chối nghiệp vụ** | hiện toast, giữ form |

### Bề mặt CMS (`/bot/v1/*`)

Lỗi là JSON phẳng `{"code", "message", "retryable"}`.

| HTTP | `code` | `retryable` | Làm gì |
|---|---|---|---|
| 400 | `invalid_body`, `empty_message`, `text_required`, `card_required` | `false` | sửa payload, **đừng** retry |
| 401 | `invalid_service_key` | `false` | kiểm `CHAT_SERVICE_KEY` hai phía |
| 404 | `not_found` | `false` | sai `groupId` / `messageId` |
| 503 | `service_key_not_configured` | `false` | Chat chưa cấu hình khoá |
| 5xx | `internal` | **`true`** | retry **cùng `Idempotency-Key`** |

`retryable` do Chat tính theo status (`>= 500`) — CMS chỉ cần đọc field này.

### WebSocket

| Tình huống | Kết quả |
|---|---|
| thiếu / sai token | **401 lúc handshake**, không lên được WS |
| không phải thành viên nhóm | kết nối OK, frame bị trả `error` `not_conversation_member` |
| `conversationId` không tồn tại | `error` `conversation_not_found` |
| JSON sai / payload sai kiểu | `error` `invalid_payload` |
| `type` không biết | `error` `unknown_frame_type` |
| gửi lại cùng `clientMsgId` | `ack` với `duplicated: true`, **không phát cho ai** |
| gửi `typing` | **không có ack** — đúng, không phải lỗi |
| không hoạt động lâu | server ping mỗi 25s; client không trả pong trong 90s thì bị đóng |
| mất kết nối | Pub/Sub là at-most-once → **gọi lại REST** `messages?cursor=` để đồng bộ |

---

## 8. Giới hạn khi chạy ở local

| Chiều | Local | Vì sao |
|---|---|---|
| Chat → CMS (`form.submit`, `card.action`, `message.created`) | **chạy** | máy bạn gọi ra Internet được |
| CMS → Chat (`POST /bot/v1/messages`…) | **không** | CMS ở Internet không thấy `localhost` của bạn |

Nên ở local: gửi form **thành công** nhưng **card không hiện trong nhóm** — vì bước CMS gọi ngược
không xảy ra. Đó là lý do mục 6 có bước "④ đóng vai CMS".

Kiểm CMS đã gọi vào Chat chưa:

```bash
make log-grep Q='"path":"/bot/v1/messages"'
```

Không có dòng nào từ IP của CMS = chưa thông. Hai cách thông:

1. **Deploy Chat lên domain công khai**, đưa CMS `CHAT_API_BASE=https://chat.<domain>/bot/v1` +
   `CHAT_SERVICE_KEY` — xem [cutover](deploy.md#cutover-bật-tích-hợp-cms).
2. **Tạm mở tunnel**: `cloudflared tunnel --url http://localhost:8080`, đưa CMS URL tunnel đó
   (đổi mỗi lần chạy lại, chỉ dùng để thử).

Kiểm bốn đường nối CMS một lượt:

```bash
make cms-check      # phải 6/6 OK
```

---

## Đọc tiếp

| Cần gì | File |
|---|---|
| Danh sách đầy đủ endpoint + tham số | [backend.md](backend.md#danh-sách-api) |
| Hợp đồng chi tiết cho FE | [frontend.md](frontend.md#hợp-đồng-fe) |
| Giao thức WebSocket đầy đủ | [frontend.md](frontend.md#giao-thức-websocket) |
| Hợp đồng cho đội CMS | [cms-goi-chat.md](cms-goi-chat.md) |
| Spec gốc do CMS đưa | [CHAT_SERVICE_INTEGRATION.md](CHAT_SERVICE_INTEGRATION.md) |
| Sửa code Chat | [source.md](source.md) |
| Đưa lên server | [deploy.md](deploy.md) |
