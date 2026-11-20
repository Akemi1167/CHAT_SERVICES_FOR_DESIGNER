# CMS gọi Chat — Bot API

> Mới vào dự án? Đọc [FLOW.md](FLOW.md) trước — nó có sơ đồ cả ba bên và giải thích
> **khi nào** CMS cần gọi Chat. File này là chi tiết từng endpoint.

Tài liệu cho **đội CMS**. Khi có gì thay đổi bên CMS (tạo đơn, đổi trạng thái, sửa card, đổi
thành viên nhóm…) thì gọi API nào của Chat, body ra sao, nhận lại gì.

Chiều ngược lại (Chat gọi CMS: `form.submit`, `card.action`, `message.created`) nằm ở mục 6 của
[CHAT_SERVICE_INTEGRATION.md](CHAT_SERVICE_INTEGRATION.md). Tài liệu này là **mục 7** của spec đó,
viết lại theo đúng những gì Chat **đã cài thật** — có chỗ Chat trả nhiều field hơn spec, đã ghi rõ.

## Nội dung

- [Quy ước chung](#quy-ước-chung)
- [Bảng tra: thay đổi gì → gọi gì](#bảng-tra-thay-đổi-gì--gọi-gì)
- [Nhóm & thành viên](#nhóm--thành-viên)
- [Định danh người dùng](#định-danh-người-dùng)
- [Tin nhắn & card](#tin-nhắn--card)
- [Thông báo](#thông-báo)
- [Trạng thái nhân sự (presence)](#trạng-thái-nhân-sự-presence)
- [Tra cứu](#tra-cứu)
- [Lỗi và retry](#lỗi-và-retry)
- [Ví dụ trọn vòng: tạo đơn đến khi xong](#ví-dụ-trọn-vòng-tạo-đơn-đến-khi-xong)

---

## Quy ước chung

**Base URL**: `CHAT_API_BASE` = `<host Chat>/bot/v1`. Local: `http://localhost:8080/bot/v1`.

**Xác thực**: mọi request thêm header

```
X-Chat-Service-Key: <CHAT_SERVICE_KEY>
```

Đây là khoá dùng chung hai phía, cùng khoá Chat dùng để ký webhook gửi sang CMS. Sai khoá →
`401 {"code":"invalid_service_key","retryable":false}`. Chat chưa cấu hình khoá →
`503 {"code":"service_key_not_configured"}`.

**Không có envelope.** Response là JSON phẳng (`{"messageId": "..."}`), khác bề mặt cho FE
(`{statusCode, code, message, element}`).

**`Idempotency-Key` (UUID) cho mọi request ghi.** Chat lưu lại `(key → status + body)`; gặp lại
key cũ thì **trả nguyên kết quả lần đầu**, không làm lại việc — kể cả khi body lần hai khác hẳn.
Nhờ vậy CMS retry an toàn: retry phải dùng **cùng key**, đổi key mới là tạo bản ghi mới.

```bash
-H "Idempotency-Key: $(uuidgen)"
```

Bảy endpoint có hiệu lực: `POST /messages`, `PATCH /messages/:id`, `POST /messages/:id/thread`,
`POST /messages/:id/pin`, `POST /notifications`, `PUT /presence`, `PUT /groups/:id`.

`POST /users/resolve` **không cần** — nó vốn đã idempotent theo `openId`: gọi bao nhiêu lần cũng
trả cùng `actorId`, không tạo thêm người.

**Id nhóm**: mọi endpoint nhận `wg_<hex>` (chatId của CMS) **hoặc** UUID nội bộ của Chat.
Ngoại lệ duy nhất là `PUT /groups/:id` — tham số đó **chính là** chatId cần tạo, nên bắt buộc
dạng `wg_<hex>`.

**Id người dùng**: `actorId` (`u_68`) là định danh Chat cấp. Các endpoint tra cứu cũng nhận
`openId` hoặc UUID nội bộ.

---

## Bảng tra: thay đổi gì → gọi gì

| Bên CMS vừa xảy ra | Gọi | Kết quả người dùng thấy |
|---|---|---|
| Tạo đơn mới | `POST /messages` (kèm `card`) | card đơn hiện trong nhóm, realtime |
| **Đổi trạng thái đơn** (nhận / tạm dừng / treo / nộp bài / xong / huỷ) | `PATCH /messages/:id` — **một request làm cả 3 việc**: đổi card + ghi thread + thông báo | card cập nhật tại chỗ, thêm dòng diễn biến, người liên quan nhận chuông |
| Chỉ ghi một dòng diễn biến | `POST /messages/:id/thread` | dòng chữ trong thread của card |
| Chỉ thông báo cho ai đó | `POST /notifications` | chuông đỏ + badge của riêng người đó |
| Đổi tên nhóm / thêm / bớt thành viên | `PUT /groups/:id` | sidebar của người liên quan tự tải lại |
| User mới chưa từng vào Chat | `POST /users/resolve` | có `actorId` để dùng trong `visibleTo` |
| Ghim / bỏ ghim card quan trọng | `POST /messages/:id/pin` | card hiện ở mục đã ghim |
| Lịch OFF / hết ca | `PUT /presence` | chữ **OFF** cạnh tên |
| Cần đối chiếu khi debug | `GET /groups`, `GET /groups/:id/members`, `GET /users/:id` | — |

Nguyên tắc: **đơn/task do CMS sở hữu**, Chat chỉ giữ card và chuyển event. Đổi trạng thái đơn thì
đừng gửi tin nhắn mới — `PATCH` lại card để lịch sử nhóm không bị ngập.

---

## Nhóm & thành viên

### `PUT /groups/:chatId` — đẩy nhóm + thành viên

Tạo nhóm nếu chưa có, cập nhật tên, và **thay toàn bộ** danh sách thành viên. Ai không còn trong
`members` sẽ **mất quyền xem nhóm ngay** (Chat set `left_at`).

```bash
curl -X PUT "$CHAT_API_BASE/groups/wg_95f6fdbfaa17593e" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Nhóm Thiết kế Web",
    "members": [
      {"openId":"local:7a8060c4-…","cmsUserId":69,"name":"Lê Trợ Lý",
       "sources":["assistant"],"teamCodes":[]},
      {"openId":"local:c6816fc4-…","cmsUserId":68,"name":"Trần Designer",
       "sources":["team"],"teamCodes":["DESIGN"]}
    ]
  }'
```

```json
{ "id": "wg_95f6fdbfaa17593e", "memberCount": 2 }
```

- `openId` là **bắt buộc** — Chat dùng nó để khớp người; `cmsUserId`, `name`, `avatarUrl` dùng để
  tạo/cập nhật hồ sơ nếu người đó chưa có trong Chat.
- `sources` (`team` / `assistant` / `manual` / `invite`) và `teamCodes` được lưu lại và trả về cho
  FE, Chat không diễn giải.
- `sources` chứa `assistant` → Chat coi là **admin trong nhóm** (chỉ để kiểm quyền nội bộ của
  Chat; quyền nghiệp vụ vẫn do CMS quyết).

**Chat chỉ bắn `reload` khi thật sự có thay đổi.** Gọi `PUT` với payload y nguyên thì không ai nhận
event — FE không tải lại vô ích:

| Thay đổi | Event `reload` · `reason` | Gửi cho ai |
|---|---|---|
| thêm / bớt thành viên | `group.members_changed` | thành viên hiện tại **∪** người vừa bị gỡ |
| đổi tên nhóm (hoặc dữ liệu nhóm) | `group.updated` | thành viên hiện tại |
| không đổi gì | *(không bắn)* | — |

Chat cũng tự `GET /workspace/groups` của CMS mỗi `CMS_SYNC_INTERVAL` (mặc định 60s), nên nhóm
cuối cùng vẫn khớp CMS kể cả khi `PUT` thất bại. `PUT` chỉ để **nhanh hơn**.

---

## Định danh người dùng

### `POST /users/resolve` — `openId` → `actorId`

Gọi khi cần `actorId` mà chưa có — điển hình là để điền `visibleTo` của nút trên card. Người chưa
từng đăng nhập Chat cũng dùng được: Chat tạo actor từ hồ sơ CMS.

```bash
curl -X POST "$CHAT_API_BASE/users/resolve" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"items":[{"openId":"local:c6816fc4-…","cmsUserId":68,"name":"Trần Designer"}]}'
```

```json
{ "items": [
  { "openId": "local:c6816fc4-…", "actorId": "u_68", "name": "Trần Designer", "presence": "on" }
] }
```

Một lần gọi nhiều người — Chat tra bằng **2 câu query** chứ không phải N câu. `actorId` ổn định
theo `cmsUserId` (`u_<cmsUserId>`), nên **CMS nên cache** và không cần gọi lại mỗi lần.

---

## Tin nhắn & card

### `POST /messages` — gửi tin / card vào nhóm

```bash
curl -X POST "$CHAT_API_BASE/messages" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{
    "groupId": "wg_95f6fdbfaa17593e",
    "bot": {"id": 1, "name": "Designer Bot"},
    "card": {
      "schema": "abc.card/v1",
      "header": {"code":"WEB-20261118-0006","kind":"DESIGN","title":"Banner Tết 2027",
                 "status":{"text":"Chưa nhận","tone":"warning"}},
      "elements": [
        {"type":"field","label":"Số lượng","value":"3"},
        {"type":"button","text":"Nhận việc","action":"accept_task",
         "visibleTo":["u_68"]}
      ]
    }
  }'
```

```json
{ "messageId": "7a5510a9-4df4-437b-a39f-8fa944c0ab93", "cardVersion": 1 }
```

- Body: `{groupId, text}` hoặc `{groupId, card}` (hoặc cả hai), kèm `threadRootId` nếu muốn gửi
  vào thread. Thiếu cả `text` và `card` → `400 empty_message`.
- `bot: {id, name}` = bot sở hữu nhóm, Chat dùng làm người gửi (`b_<id>`). Bỏ trống thì dùng bot
  mặc định.
- Nhóm không tồn tại → `404 {"code":"not_found"}`.

> ⚠️ **Khoá của nút là `action`, không phải `actionId`.** Dùng sai tên thì card vẫn tạo được nhưng
> người bấm sẽ nhận `403 action_not_allowed` — vì Chat không tìm thấy action nào khớp.

**`visibleTo`**: danh sách `actorId` được thấy nút đó. Chat **lọc nút theo từng người xem** trước
khi gửi xuống UI, và **chặn** action không nằm trong `visibleTo`. Để `null` / bỏ trống = ai cũng
thấy. Mỗi người nhận một bản card đã lọc riêng, kể cả qua realtime.

### `PATCH /messages/:messageId` — sửa card (đổi trạng thái đơn)

API dùng **nhiều nhất** sau khi đơn đã tạo. Gọi **một lần** là xong cả ba việc hay đi cùng nhau.

```bash
curl -X PATCH "$CHAT_API_BASE/messages/7a5510a9-…" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{
    "card": {"schema":"abc.card/v1",
             "header":{"code":"WEB-20261118-0006","title":"Banner Tết 2027",
                       "status":{"text":"Đã nộp bài","tone":"success"}},
             "elements":[{"type":"text","text":"Chờ duyệt"}]},
    "cardVersion": 3,

    "thread": {"text":"Trần Designer đã nộp bài · Chờ duyệt",
               "bot":{"id":1,"name":"Designer Bot"}},

    "notify": [{"actorId":"u_68","text":"Đơn WEB-20261118-0006 đã nộp bài"}]
  }'
```

```json
{
  "ok": true,
  "cardVersion": 3,
  "thread": { "messageId": "bba88da4-b9ce-46c8-b8e9-2bcc6adc4a8f" },
  "notify": [ { "id": "154935fa-…", "actorId": "u_68", "text": "…",
                "messageId": "7a5510a9-…", "createdAt": 1795146120060, "read": false } ]
}
```

`card` + `cardVersion` là **bắt buộc**. `thread` và `notify` **tuỳ chọn** — không gửi thì
response không có khoá đó, hành vi y như trước khi có tính năng này. `notify` là mảng, gửi
nhiều người một lần được; `notify[].messageId` bỏ trống thì tự trỏ về chính card đang sửa.

Tương đương 3 request cũ, nhưng **một** `Idempotency-Key` và **một** chỗ xử lý lỗi:

```
PATCH /messages/:id   {card, cardVersion}
POST  /messages/:id/thread   {text}            ← gộp vào field `thread`
POST  /notifications  {actorId, text}          ← gộp vào field `notify`
```

Hai endpoint cũ **vẫn còn**, dùng khi chỉ cần một việc lẻ.

#### `cardVersion` — chống ghi đè ngược

Chat chỉ áp card mới khi `cardVersion` **lớn hơn** giá trị đang lưu. Nhỏ hơn hoặc bằng → bỏ qua:

```json
{ "ok": true, "ignored": true, "cardVersion": 5 }
```

`ignored: true` **không phải lỗi** — nghĩa là Chat đang giữ bản mới hơn (một `PATCH` khác tới
trước). Khi đó Chat **bỏ qua luôn `thread` và `notify`**: CMS đang gửi trạng thái cũ, ghi thêm
dòng diễn biến cũ vào nhóm là sai. Nhìn response thấy `ignored: true` mà không có khoá
`thread`/`notify` là đúng.

Vì vậy `cardVersion` phải **tăng đơn điệu** theo mỗi lần đổi đơn.

#### Lỗi của phần phụ

Card là việc chính. Nếu card đổi xong mà `thread` hoặc `notify` lỗi, Chat **vẫn trả `200`** và
báo lỗi theo từng phần:

```json
{ "ok": true, "cardVersion": 4,
  "notify": [
    { "id": "adcccdbe-…", "actorId": "u_6", "text": "…" },
    { "error": { "code": "user_not_found", "message": "Khong tim thay nguoi dung" } }
  ] }
```

```json
{ "ok": true, "cardVersion": 5,
  "thread": { "error": { "code": "text_required", "message": "Thieu text" } } }
```

Vì sao không trả 5xx: card **đã** đổi rồi. CMS retry sẽ gặp `ignored: true` (vì `cardVersion`
không tăng nữa) và mất luôn cả `thread`/`notify` — tệ hơn là biết phần nào lỗi.

CMS nên đọc: `ignored` trước, rồi `thread.error` / `notify[].error` nếu có.

Sau khi áp, Chat phát realtime `message` cho cả nhóm (card đã lọc theo từng người) — FE **upsert
theo `id`**, không tạo bong bóng tin nhắn mới.

### `POST /messages/:messageId/thread` — ghi một dòng diễn biến

```bash
curl -X POST "$CHAT_API_BASE/messages/7a5510a9-…/thread" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"text":"Đã giao Trần Designer · Chờ nhận task.","bot":{"id":1,"name":"Designer Bot"}}'
```

```json
{ "messageId": "a8e003fb-eccb-453b-bb83-54416e589d5b" }
```

Dùng cho lịch sử đơn: *"… đã nhận task"*, *"… đã nộp bài · Chờ duyệt"*, *"Phiếu sửa #1 · …"*,
*"Gọi hỗ trợ · …"*, *"… đã huỷ yêu cầu"*. Thiếu `text` → `400 text_required`.

### `POST /messages/:messageId/pin` — ghim / bỏ ghim

```bash
curl -X POST "$CHAT_API_BASE/messages/7a5510a9-…/pin" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" -d '{}'
```

```json
{ "ok": true, "messageId": "7a5510a9-…", "pinned": true }
```

`{"pinned": false}` để bỏ ghim. Body rỗng `{}` = ghim.

---

## Thông báo

### `POST /notifications` — thông báo cho một người

```bash
curl -X POST "$CHAT_API_BASE/notifications" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"actorId":"u_68",
       "text":"Bạn có task mới: WEB-20261118-0006 · Banner Tết 2027",
       "messageId":"7a5510a9-…"}'
```

```json
{ "id": "n_148", "actorId": "u_68",
  "text": "Bạn có task mới: …", "messageId": "7a5510a9-…",
  "createdAt": 1794976774706, "read": false }
```

`messageId` để FE bấm vào thông báo là nhảy tới đúng card. Chat phát realtime `notification` cho
riêng người đó và tăng `unreadNotifications`. Người dùng tự đánh dấu đã đọc qua bề mặt FE — CMS
không cần gọi gì thêm.

---

## Trạng thái nhân sự (presence)

### `PUT /presence` — nhiều người cùng lúc

```bash
curl -X PUT "$CHAT_API_BASE/presence" \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" \
  -H "Content-Type: application/json" \
  -d '{"items":[
        {"actorId":"u_68","status":"off","reason":"Lịch OFF ca chiều"},
        {"actorId":"u_69","status":"on"}
      ]}'
```

```json
{ "updated": 2 }
```

| `status` | `reason` | Chat hiểu là |
|---|---|---|
| `on` | — | online |
| `off` | **có** | `off_duty` → FE hiện chữ **OFF** cạnh tên |
| `off` | không | offline (chỉ là đang không mở app) |

`actorId` không tồn tại thì **bị bỏ qua, không báo lỗi** — `updated` cho biết thực sự đổi được bao
nhiêu. Chat phát realtime `presence` cho những người **chung nhóm** với người đó.

---

## Tra cứu

Dùng khi debug hoặc đối chiếu đồng bộ, không cần cho luồng chính.

| API | Trả về |
|---|---|
| `GET /groups?q=&source=` | `{items:[{id, internalId, name, source, memberCount, cms}]}` — `q` tìm theo tên, `source` = `cms` \| `local` |
| `GET /groups/:id` | một nhóm, cùng hình dạng trên |
| `GET /groups/:id/members` | `{items:[{id, name, title, presence, avatarUrl, role}]}` — `id` là **actorId** |
| `GET /users/:id` | một user (nhận `actorId` \| `openId` \| UUID) |
| `POST /users/batch` | `{actorIds:[], openIds:[], cmsUserIds:[]}` → `{items:[…]}`, mỗi loại id **một câu query** |
| `GET /attachments/:id` | metadata tệp; `?redirect=true` → `302` tới URL thật |

Chat **không lưu file**, chỉ lưu URL do service upload/CDN cung cấp.

---

## Lỗi và retry

Lỗi trả JSON phẳng:

```json
{ "code": "not_found", "message": "Khong tim thay nhom", "retryable": false }
```

| HTTP | `code` hay gặp | `retryable` | CMS nên làm |
|---|---|---|---|
| 400 | `invalid_body`, `empty_message`, `text_required`, `card_required`, `invalid_request` | `false` | sửa payload, **đừng** retry |
| 401 | `invalid_service_key` | `false` | kiểm `CHAT_SERVICE_KEY` hai phía |
| 403 | `action_not_allowed` | `false` | action không có trong `visibleTo` |
| 404 | `not_found` | `false` | sai `groupId` / `messageId` |
| 503 | `service_key_not_configured` | `false` | Chat chưa cấu hình khoá |
| 5xx | `internal` | **`true`** | retry **cùng `Idempotency-Key`** |

`retryable` do Chat tính theo status (`>= 500`), nên CMS chỉ cần đọc field này chứ không phải
map từng mã lỗi.

Quy tắc retry: lỗi mạng / timeout / 5xx → thử lại **một lần** với **cùng key**. Idempotency đảm
bảo không tạo card hay thông báo trùng.

---

## Ví dụ trọn vòng: tạo đơn đến khi xong

```
① FE bấm "Tạo yêu cầu"
   FE ──POST /api/client/v1/chat/forms/create_task/submit──▶ Chat
   Chat ──webhook form.submit (HMAC)──▶ CMS            ← đồng bộ, CMS phải trả lời
   CMS tạo đơn WEB-20261118-0006
   CMS ──{ok:true, toast:"Đã tạo đơn"}──▶ Chat ──▶ FE

② CMS đẩy card vào nhóm
   CMS ──POST /bot/v1/messages {groupId, card}──▶ Chat
        ◀── {messageId:"7a55…", cardVersion:1}
   → lưu cặp (messageId ↔ mã đơn) để lần sau PATCH

③ CMS thông báo cho người được giao
   CMS ──POST /bot/v1/notifications {actorId:"u_68", messageId:"7a55…"}──▶ Chat

④ Người đó bấm "Nhận việc" trên card
   FE ──POST /api/client/v1/chat/messages/7a55…/actions {action:"accept_task"}──▶ Chat
   Chat kiểm visibleTo → ──webhook card.action──▶ CMS
   CMS đổi trạng thái đơn, trả card mới ──▶ Chat áp card, phát cho cả nhóm

⑤ Mỗi lần trạng thái đơn đổi (tạm dừng, nộp bài, xong…)
   CMS ──PATCH /bot/v1/messages/7a55… ──▶ Chat     ← MỘT request, ba việc
        {card, cardVersion: n+1,
         thread: {text:"… đã nộp bài"},
         notify: [{actorId:"u_68", text:"…"}]}

⑥ Thành viên nhóm đổi
   CMS ──PUT /bot/v1/groups/wg_95f6… {name, members}──▶ Chat
   → Chat bắn reload CHỈ khi danh sách thật sự đổi
```

Hai điều cần nhớ ở bước ②: **lưu `messageId`** mà Chat trả về (cặp với mã đơn) — không có nó thì
không `PATCH` được card về sau. Và **`cardVersion` phải tăng** mỗi lần đổi, nếu không `PATCH` sẽ bị
bỏ qua với `ignored: true`.

---

## Thử nhanh

Collection Postman có sẵn folder `08 · CMS → Chat (Bot API)` với đủ 14 request này:

```bash
make postman-run-cms TOKEN=<access token CMS> KEY=<CHAT_SERVICE_KEY> CHAT_ID=wg_...
```

Kiểm Chat đã bật tích hợp chưa và khoá hai phía có khớp:

```bash
make cms-check
```

Cấu hình phía Chat (`CMS_ENABLED`, `CHAT_SERVICE_KEY`, `CMS_WEBHOOK_URL`…):
[deploy.md](deploy.md#cutover-bật-tích-hợp-cms).
