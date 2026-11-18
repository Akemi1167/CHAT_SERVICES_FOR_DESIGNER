# Tích hợp Chat Service (Go) ↔ CMS

Tài liệu dành cho đội làm **Chat Service** (service Go riêng). Mô tả kiến trúc, các luồng, cách xác thực, API CMS cung cấp, event Chat phải gửi, Bot API Chat phải có, và dữ liệu CMS đang cung cấp. Bản tham chiếu đang chạy: `tools/chat-mock/server.mjs` (mock đóng vai Chat Service). Payload trong tài liệu là payload thật lấy từ hệ thống dev.

> API cho giao diện web chat: CMS `docs/WEB_CHAT_UI_API.md` · Chat Service `docs/CHAT_SERVICE_UI_API.md`. Kế hoạch / trạng thái: `docs/PLAN_WEB_APP_API.md`.

## Mục lục

1. [Kiến trúc & nguyên tắc](#1-kiến-trúc--nguyên-tắc)
2. [Cấu hình hai phía](#2-cấu-hình-hai-phía)
3. [Xác thực](#3-xác-thực)
4. [Các luồng](#4-các-luồng)
5. [API CMS cung cấp cho Chat Service](#5-api-cms-cung-cấp-cho-chat-service)
6. [Event Chat Service gửi CMS (webhook)](#6-event-chat-service-gửi-cms-webhook)
7. [Bot API Chat Service phải có (CMS gọi)](#7-bot-api-chat-service-phải-có-cms-gọi)
8. [Dữ liệu CMS đang cung cấp](#8-dữ-liệu-cms-đang-cung-cấp)
9. [Client API Chat Service cung cấp cho UI](#9-client-api-chat-service-cung-cấp-cho-ui)
10. [Checklist triển khai](#10-checklist-triển-khai)

---

## 1. Kiến trúc & nguyên tắc

```
                    (1) REST + WebSocket/SSE
   ┌──────────┐  ◄────────────────────────►  ┌──────────────────┐
   │  Web UI  │                              │ Chat Service (Go)│
   │          │                              │ nhóm, tin, card, │
   └────┬─────┘                              │ realtime, presence│
        │                                    └──┬────▲─────┬────┘
        │ (4) Đăng nhập + Workspace API (CMS phát │    │     │ (5) Workspace API
        │     JWT; Chat kiểm bằng JWKS của CMS)  │    │     │
        │                                        │(2) │(3)  │     (service key)
        ▼                                        ▼    │     ▼
   ┌─────────────────────────────────────────────────┴──────────┐
   │  CMS / Task Engine (Node)                                   │
   │  user, nhóm Web, Tổ (Web), định mức, đơn Web, quyền, SLA     │
   └─────────────────────────────────────────────────────────────┘
```

| Luồng | Hướng | Xác thực | Nội dung |
|---|---|---|---|
| (1) | UI ↔ Chat | **access token do CMS phát** (Chat kiểm tra bằng JWKS của CMS) | bootstrap, tin nhắn, bấm nút, form, realtime |
| (2) | Chat → CMS | HMAC `X-Abc-Signature` (ký bằng khoá dùng chung) | event `form.submit`, `card.action`, `message.created`, `user.updated` |
| (3) | CMS → Chat | `X-Chat-Service-Key` (khoá dùng chung) | gửi / sửa card, trả lời thread, thông báo, resolve user, đẩy thành viên nhóm |
| (4) | UI → CMS | không (login / refresh / đổi mật khẩu) · access token (còn lại) | **đăng nhập, làm mới token, đăng xuất, đổi mật khẩu**, dữ liệu nghiệp vụ (tổ, gợi ý, định mức, đơn, link mời) |
| (5) | Chat → CMS | `X-Chat-Service-Key` | danh sách nhóm + thành viên, hồ sơ user, (tuỳ chọn) đọc tổ / định mức / gợi ý / đơn |

**Nguyên tắc**

- Chat Service **không đọc database của CMS**. Mọi dữ liệu user / nhóm / thành viên lấy qua API (luồng 5).
- Chat Service **không chứa nghiệp vụ đơn**. Nó lưu tin / card, lọc nút theo `visibleTo`, chuyển event sang CMS.
- CMS **không giữ kết nối realtime** và không biết ai online. Mọi thay đổi hiển thị đi qua Bot API (luồng 3) → Chat phát xuống UI.
- **CMS quản lý tài khoản và phiên đăng nhập**: UI đăng nhập ở CMS, CMS phát access token (JWT RS256) dùng chung cho CMS và Chat. Chat Service **không xử lý mật khẩu / phiên**, chỉ kiểm tra token.
- CMS là nguồn sự thật cho: tài khoản, phiên đăng nhập, nhóm Web, thành viên nhóm, quyền, đơn. Chat là nguồn sự thật cho: `actorId`, tin nhắn, `cardVersion`, trạng thái đọc, presence.

---

## 2. Cấu hình hai phía

**Phía CMS** (`.env`)

| Biến | Ví dụ dev | Ý nghĩa |
|---|---|---|
| `CHAT_SERVICE_KEY` | `mock_service_key` | **khoá dùng chung duy nhất** CMS ↔ Chat: ký / kiểm HMAC webhook (luồng 2), CMS gọi Bot API (luồng 3), Chat gọi Workspace API (luồng 5) |
| `CHAT_API_BASE` | `http://localhost:4010/bot/v1` | base Bot API của Chat |
| `WEB_APP_ORIGINS` | `http://localhost:4010` | CORS cho UI gọi CMS (trống = `*` khi không phải production) |
| `ENABLE_WEB_QUEUE_SWEEPER` | `1` | bật / tắt bộ tự giao đơn xếp hàng (chu kỳ theo bot: CMS → Cài đặt → `WEB_QUEUE_SWEEP_SECONDS`) |

Kênh Web là **một loại nhóm dưới bot CMS sẵn có** (như một category), nên không có bot app id / secret riêng. Bot CMS được xác định bằng `webhookPath`; tên bot được CMS gửi kèm mỗi tin bot (`bot: {id, name}`) để Chat hiển thị người gửi.

**Phía Chat Service** (tên biến tuỳ đội Go — giá trị phải khớp)

| Cấu hình | Giá trị dev |
|---|---|
| URL webhook CMS (theo `webhookPath` của bot) | `http://localhost:3001/webhook/web/designer` |
| Base Workspace API CMS | `http://localhost:3001/api/client/v1/workspace` |
| Khoá dùng chung (ký webhook, gọi Workspace API, kiểm request Bot API của CMS) | = `CHAT_SERVICE_KEY` |
| JWKS của CMS (kiểm access token) | `<CMS>/api/client/v1/workspace/.well-known/jwks.json` |
| JWT `iss` / `aud` phải kiểm | `designer-cms` / `abc-workspace` |

---

## 3. Xác thực

Toàn bộ kết nối service ↔ service dùng **một khoá**: `CHAT_SERVICE_KEY`.

### 3.1 Service key (Chat → CMS)

Header `X-Chat-Service-Key: <CHAT_SERVICE_KEY>` cho các API ở [mục 5](#5-api-cms-cung-cấp-cho-chat-service). Sai / thiếu → `401 UNAUTHORIZED`. CMS chưa cấu hình → `503 SERVICE_KEY_NOT_CONFIGURED`.

### 3.2 Chữ ký webhook (Chat → CMS)

```
X-Abc-Timestamp: <unix giây>
X-Abc-Signature: hex( HMAC_SHA256( CHAT_SERVICE_KEY, "<timestamp>.<rawBody>" ) )
```

- Ký trên **raw body đúng như gửi đi** (không serialize lại).
- Lệch giờ tối đa **5 phút**. Sai chữ ký → `401`.
- Body có `schema: "abc.event/v1"` và `eventId` duy nhất. CMS **chống trùng theo `eventId`** — gửi lại cùng `eventId` nhận `{"ok":true,"deduped":true}`.

Ví dụ Go:

```go
mac := hmac.New(sha256.New, []byte(serviceKey)) // = CHAT_SERVICE_KEY
mac.Write([]byte(ts + "." + string(rawBody)))
sig := hex.EncodeToString(mac.Sum(nil))
```

### 3.3 Access token do CMS phát (UI dùng cho cả CMS và Chat)

UI đăng nhập ở CMS (`POST /api/client/v1/workspace/auth/login`) và nhận `accessToken` (JWT RS256, 30 phút) + `refreshToken` (30 ngày, đổi mới mỗi lần làm mới). Chi tiết API: `docs/WEB_CHAT_UI_API.md` mục 1.

UI gửi `Authorization: Bearer <accessToken>` tới Chat Service (REST và realtime). **Chat Service kiểm tra:**

| | Giá trị |
|---|---|
| JWKS | `GET <CMS>/api/client/v1/workspace/.well-known/jwks.json` — cache vài giờ; gặp `kid` lạ thì tải lại |
| Thuật toán | `RS256`, chọn khoá theo `kid` trong header |
| `iss` / `aud` | `designer-cms` / `abc-workspace` |
| `typ` | `"access"` |
| `exp` | còn hạn |
| Định danh | `cmsUserId` (số, = `users.id`), `openId`, `name`; `sub` = `cmsUserId` dạng chuỗi |

Payload mẫu (đã giải mã):

```json
{ "sub": "92", "cmsUserId": 92, "openId": "local:62742805-c0dd-4da5-b249-26707361d008", "name": "Nguyễn Order",
  "typ": "access", "iss": "designer-cms", "aud": "abc-workspace", "iat": 1794991846, "exp": 1794993646 }
```

JWKS mẫu:

```json
{ "keys": [ { "kty": "RSA", "n": "okdjtcFSNn3lROaF…", "e": "AQAB", "kid": "cms-20261118-5c018789", "alg": "RS256", "use": "sig" } ] }
```

- Lần đầu gặp `cmsUserId` → Chat tạo actor (hồ sơ qua `GET /workspace/users?ids=`), đồng bộ nhóm.
- Token hết hạn / sai → Chat trả `401`; UI tự làm mới ở CMS rồi gọi lại.
- Khoá user / đổi mật khẩu trên CMS → CMS thu hồi refresh token ngay; access token đang có hết hạn trong tối đa 30 phút (CMS từ chối ngay ở API của mình).
- Ví dụ Go: `github.com/MicahParks/keyfunc` (JWKS) + `github.com/golang-jwt/jwt/v5` với `jwt.WithIssuer("designer-cms")`, `jwt.WithAudience("abc-workspace")`, `jwt.WithValidMethods([]string{"RS256"})`.

### 3.4 Bot API (CMS → Chat)

CMS gửi `X-Chat-Service-Key: <CHAT_SERVICE_KEY>` trên **mọi** request Bot API (không có bước lấy token). Chat so sánh constant-time với khoá đã cấu hình; sai → `401 {"code":"invalid_service_key","message":"X-Chat-Service-Key không hợp lệ","retryable":false}`.

---

## 4. Các luồng

### 4.1 Đăng nhập (ở CMS)

```
UI ──POST /workspace/auth/login {identifier,password}──► CMS
     200 { accessToken, refreshToken, user }
     403 PASSWORD_CHANGE_REQUIRED → UI ──POST /workspace/auth/change-password──► CMS → đăng nhập lại
     401 INVALID_CREDENTIALS / 403 USER_INACTIVE
UI ──GET /chat/bootstrap  (Bearer accessToken)──► Chat
     Chat kiểm token bằng JWKS của CMS → actor u_<cmsUserId>
     (lần đầu) Chat ──GET /workspace/users?ids=<cmsUserId>──► CMS   (hồ sơ: tổ Web, trợ lý, admin bot → vai trò hiển thị)
               Chat ──GET /workspace/groups──► CMS                  (nhóm + thành viên)
UI ══ realtime (Bearer accessToken / ?token=) ══► Chat
```

### 4.2 Làm mới token / đăng xuất

```
UI (access token sắp hết hoặc nhận 401) ──POST /workspace/auth/refresh {refreshToken}──► CMS
     200 { accessToken mới, refreshToken mới }   → UI mở lại realtime với token mới
     401 INVALID_REFRESH_TOKEN / 403 USER_INACTIVE → UI về màn đăng nhập
UI ──POST /workspace/auth/logout {refreshToken}──► CMS
```

### 4.3 Đồng bộ nhóm & thành viên

Nhóm Web do admin tạo trên CMS (chatId dạng `wg_…`). Chat **không tự tạo nhóm**.

```
(a) Chat poll:      Chat ──GET /workspace/groups (service key)──► CMS      (khi khởi động + định kỳ, mock: 5 giây)
(b) CMS đẩy ngay:   CMS ──PUT /bot/v1/groups/:chatId {name, members[]}──► Chat
                    khi: thêm/gỡ thành viên tay, user vào bằng link mời, đổi tổ của nhóm,
                         đổi thành viên / trạng thái Tổ (Web), thêm/sửa/gỡ trợ lý
```

**Quy tắc thành viên nhóm Web** (CMS tính, Chat chỉ áp dụng):

| `sources` | Nghĩa |
|---|---|
| `team` | thành viên đang hoạt động của Tổ (Web) được chọn cho nhóm (`teamCodes` cho biết tổ nào) |
| `assistant` | trợ lý của nhóm |
| `manual` | admin thêm tay trên CMS |
| `invite` | user tự vào bằng link mời |

Một người có thể có nhiều nguồn. User bị khoá / xoá không có trong danh sách. Chat đặt `memberIds` của nhóm = danh sách CMS trả (thay thế, không cộng dồn). Người không còn trong danh sách phải mất quyền xem nhóm.

### 4.4 Vào nhóm bằng link mời

```
Admin CMS tạo link:  <WEB_APP_URL>?join=<token>      (WEB_APP_URL = CMS → Bot → Cài đặt, không phải env)
UI mở link → (đăng nhập) ──GET /workspace/invites/:token (JWT)──► CMS   (xem trước)
UI ──POST /workspace/groups/join {token} (JWT)──► CMS
     CMS thêm thành viên (nguồn invite) ──PUT /bot/v1/groups/:chatId──► Chat   (đẩy danh sách mới)
UI ──POST /chat/groups/refresh──► Chat ──GET /workspace/groups──► CMS          (đảm bảo đã đồng bộ)
UI mở nhóm
```

### 4.5 Tạo yêu cầu (đơn)

```
UI ──POST /chat/forms/create_task/submit {values, context.groupId}──► Chat
     Chat ──webhook form.submit──► CMS
          CMS kiểm tra: user hợp lệ, là thành viên nhóm, tổ thuộc nhóm
          CMS tạo đơn web_orders (mã WEB-YYYYMMDD-NNNN), chọn người nhận / xếp hàng
          CMS ──POST /bot/v1/users/resolve {openId}──► Chat → actorId (người nhận, admin, trợ lý)
          CMS ──POST /bot/v1/messages {groupId, card}──► Chat → {messageId, cardVersion:1}
          CMS ──POST /bot/v1/messages/:id/thread {text:"Đã giao … · Chờ nhận task."}──► Chat
          CMS ──POST /bot/v1/notifications {actorId, text, messageId}──► Chat
     CMS ──200 {ok, toast, closeForm}──► Chat ──► UI
```

Payload thật: [6.2](#62-formsubmit--create_task).

### 4.6 Bấm nút trên card

```
UI ──POST /chat/messages/:id/actions {action, values}──► Chat
     Chat kiểm tra action có trong card và actor ∈ visibleTo (không → 403 cho UI)
     Chat ──webhook card.action {messageId, action, values, actor, cardVersion}──► CMS
          CMS kiểm tra quyền + trạng thái, cập nhật đơn (khoá lạc quan)
          CMS ──POST /bot/v1/messages/:id/thread──► Chat   (dòng hệ thống)
          CMS ──POST /bot/v1/notifications──► Chat         (nếu cần)
     CMS ──200 {ok, toast, closeForm, card, cardVersion}──► Chat
     Chat thay card nếu cardVersion > hiện tại → phát realtime cho cả nhóm → trả {toast, closeForm} cho UI
```

- Lỗi nghiệp vụ: CMS trả `200` + `toast.type: "error"`, `closeForm: false`, **không có `card`**.
- Hai người bấm cùng lúc: người sau nhận toast "Task vừa được người khác cập nhật — thử lại".

### 4.7 Xếp hàng & tự giao (CMS chủ động)

Đơn không có người rảnh / ngoài giờ → `queued`. CMS tự giao định kỳ (60 giây) và ngay sau các thao tác làm thay đổi tải:

```
CMS ──PATCH /bot/v1/messages/:id {card, cardVersion}──► Chat   (card "Chưa nhận" có nút Nhận task)
CMS ──POST /bot/v1/messages/:id/thread {text:"Tự giao … · Chờ nhận task."}──► Chat
CMS ──POST /bot/v1/notifications──► Chat
```

### 4.8 Định mức thay đổi

Admin thêm / sửa / bật-tắt định mức của Tổ (Web) → CMS dựng lại card các đơn "Chưa nhận" của tổ và `PATCH /bot/v1/messages/:id` (form "Nhận task" luôn đúng danh sách định mức).

### 4.9 Tin nhắn thường

```
UI ──POST /chat/groups/:id/messages──► Chat (lưu, phát realtime) ──webhook message.created──► CMS
```

CMS hiện chỉ ghi nhật ký event này. Gửi bất đồng bộ, không chặn UI.

---

## 5. API CMS cung cấp cho Chat Service

Base: `$CMS/api/client/v1/workspace`. Response bọc `{ statusCode, code, message, element }`.

### 5.1 JWKS — `GET /.well-known/jwks.json`

**Mô tả.** Khoá công khai để Chat Service kiểm tra access token do CMS phát ([3.3](#33-access-token-do-cms-phát-ui-dùng-cho-cả-cms-và-chat)). Không cần xác thực. CMS tự sinh khoá RSA ở lần khởi động đầu (lưu trong MongoDB), không cần cấu hình.

**CURL**

```bash
curl "$CMS/api/client/v1/workspace/.well-known/jwks.json"
```

**Response** `200` (không bọc envelope — chuẩn JWKS)

```json
{ "keys": [ { "kty": "RSA", "n": "okdjtcFSNn3lROaFvGrILW8BnNgnaHYDSGfu1GYb…", "e": "AQAB", "kid": "cms-20261118-5c018789", "alg": "RS256", "use": "sig" } ] }
```

### 5.2 Đăng nhập / đổi mật khẩu — UI gọi thẳng CMS

Chat Service **không** xử lý mật khẩu. Đăng nhập, đổi mật khẩu, làm mới token, đăng xuất do UI gọi thẳng CMS: `docs/WEB_CHAT_UI_API.md` mục 1. (API `POST /auth/verify` cũ đã bỏ.)

### 5.3 Danh sách nhóm Web — `GET /groups`

**Mô tả.** Mọi nhóm Web (của các bot còn tồn tại) kèm tổ, trợ lý và **thành viên hiệu lực**. Chat dùng để tạo / cập nhật nhóm và `memberIds`.

**CURL**

```bash
curl "$CMS/api/client/v1/workspace/groups" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY"
```

**Response** `200`

```json
{
  "statusCode": 200, "code": "SUCCESS", "message": "Success",
  "element": {
    "groups": [
      {
        "chatId": "wg_95f6fdbfaa17593e", "name": "Nhóm Thiết kế Web", "botId": 1,
        "timezone": "Asia/Ho_Chi_Minh", "isOrderHub": true, "orderTeamCodes": ["DESIGN", "VIDEO"],
        "teams": [ { "id": 27, "code": "DESIGN", "name": "Thiết kế" }, { "id": 28, "code": "VIDEO", "name": "Video" } ],
        "assistants": [ { "cmsUserId": 69, "openId": "local:7a8060c4-…", "name": "Lê Trợ Lý" } ],
        "members": [
          { "cmsUserId": 69, "openId": "local:7a8060c4-…", "name": "Lê Trợ Lý", "avatarUrl": "", "sources": ["assistant"], "teamCodes": [] },
          { "cmsUserId": 67, "openId": "local:4d94607a-…", "name": "Nguyễn Order", "avatarUrl": "", "sources": ["manual"], "teamCodes": [] },
          { "cmsUserId": 68, "openId": "local:c6816fc4-…", "name": "Trần Designer", "avatarUrl": "", "sources": ["team"], "teamCodes": ["DESIGN"] }
        ]
      }
    ]
  }
}
```

### 5.4 Thành viên một nhóm — `GET /groups/:chatId/members`

**Mô tả.** Như `members` ở 5.3 nhưng cho một nhóm.

**CURL**

```bash
curl "$CMS/api/client/v1/workspace/groups/wg_95f6fdbfaa17593e/members" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY"
```

**Response** `200`

```json
{
  "statusCode": 200, "code": "SUCCESS", "message": "Success",
  "element": {
    "chatId": "wg_95f6fdbfaa17593e",
    "members": [
      { "cmsUserId": 68, "openId": "local:c6816fc4-…", "name": "Trần Designer", "avatarUrl": "", "sources": ["team"], "teamCodes": ["DESIGN"] }
    ]
  }
}
```

Lỗi: `404 GROUP_NOT_FOUND` (không có / không phải nhóm Web).

### 5.5 Hồ sơ user — `GET /users?ids=&openIds=`

**Mô tả.** Tra hồ sơ theo `ids` (cmsUserId) và / hoặc `openIds`, phân cách dấu phẩy, tối đa 200. Dùng khi: tạo actor cho người chưa đăng nhập (CMS gọi `users/resolve`), làm mới tên / avatar, suy vai trò hiển thị. User đã xoá không trả về.

**CURL**

```bash
curl "$CMS/api/client/v1/workspace/users?ids=68,69" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY"
```

**Response** `200`

```json
{
  "statusCode": 200, "code": "SUCCESS", "message": "Success",
  "element": {
    "users": [
      {
        "cmsUserId": 68, "openId": "local:c6816fc4-…", "name": "Trần Designer",
        "username": "tran.designer", "email": "tran.designer@example.com", "avatarUrl": "",
        "status": "active", "role": "user", "isDesigner": true,
        "webTeams": [ { "botId": 1, "teamId": 27, "code": "DESIGN" } ],
        "assistantOf": [], "botAdminOf": []
      },
      {
        "cmsUserId": 69, "openId": "local:7a8060c4-…", "name": "Lê Trợ Lý",
        "username": "le.troly", "email": "le.troly@example.com", "avatarUrl": "",
        "status": "active", "role": "user", "isDesigner": false,
        "webTeams": [], "assistantOf": ["wg_95f6fdbfaa17593e"], "botAdminOf": []
      }
    ]
  }
}
```

Vai trò hiển thị gợi ý (mock đang dùng): `role = "admin"` hoặc `botAdminOf` khác rỗng → Admin · TL; `assistantOf` khác rỗng → Trợ lý; `webTeams` khác rỗng → Designer; còn lại → Order. **Chỉ để hiển thị** — quyền thao tác luôn do CMS quyết định.

`avatarUrl` có thể là đường dẫn tương đối của CMS (`/api/avatars/…`) → Chat ghép với base URL CMS trước khi trả cho UI; chuỗi rỗng = không có avatar.

### 5.6 API dùng chung với UI

Chat gọi được các API đọc trong `docs/WEB_CHAT_UI_API.md` mục 2–4 + phụ lục (`teams`, `suggest-assignee`, `teams/:id/norms`, `orders`, `orders/:code`) bằng **`X-Chat-Service-Key`** (thay cho token người dùng) hoặc chuyển tiếp access token của người dùng.

---

## 6. Event Chat Service gửi CMS (webhook)

`POST <URL webhook bot>` (dev: `http://localhost:3001/webhook/web/designer`), header chữ ký ở [3.2](#32-chữ-ký-webhook-chat--cms). CMS luôn trả `200` khi chữ ký hợp lệ; kết quả nghiệp vụ nằm trong body.

### 6.1 Khung chung

```json
{
  "schema": "abc.event/v1",
  "eventId": "evt_a9ab9744-4a4a-4982-a575-c1b4871613e1",
  "type": "form.submit",
  "botId": "b_1",
  "createdAt": "2026-11-18T04:39:34.688Z",
  "...": "trường riêng của từng type"
}
```

`botId` trong event là id bot phía Chat — CMS **không dùng**, CMS xác định bot theo URL webhook. `actor` (khi có) = người thao tác: `{ "id": "<actorId>", "openId": "<users.openId>", "cmsUserId": <users.id>, "name": "…" }` — **bắt buộc có `cmsUserId` hoặc `openId`** để CMS nhận ra user.

Response CMS (đồng bộ, Chat chuyển cho UI):

| Trường | Ý nghĩa |
|---|---|
| `ok` | `true` = đã xử lý (kể cả khi nghiệp vụ từ chối) |
| `toast` | `{ type: "success"|"error"|"warning"|"info", text }` — hiện cho người thao tác |
| `closeForm` | `false` = giữ form mở (lỗi nhập liệu) |
| `card`, `cardVersion` | (chỉ `card.action` thành công) card mới để thay vào tin `messageId` |
| `deduped` | `true` = `eventId` đã xử lý trước đó, bỏ qua |

Lỗi hệ thống phía CMS: `200 {"ok":false,"toast":{"type":"error","text":"CMS lỗi khi xử lý thao tác — thử lại sau"},"closeForm":false}`.

### 6.2 `form.submit` · `create_task`

**Mô tả.** Người dùng gửi form "Yêu cầu mới". Đồng bộ (UI chờ kết quả), timeout khuyến nghị 5 giây, **không tự retry**.

**CURL**

```bash
BODY='{"schema":"abc.event/v1","eventId":"evt_a9ab9744-4a4a-4982-a575-c1b4871613e1","type":"form.submit","botId":"b_1","createdAt":"2026-11-18T04:39:34.688Z","formId":"create_task","values":{"kind":"task","title":"Banner Tết 2027","brief":"Kích thước 1200x628, tông đỏ vàng","teamId":"27","useAI":false,"assigneeOpenId":"local:c6816fc4-f523-46b3-8893-bd9b43e6abbc","assigneeName":"Trần Designer"},"actor":{"id":"u_67","openId":"local:4d94607a-27c0-4149-be89-2fafc4875fa8","cmsUserId":67,"name":"Nguyễn Order"},"context":{"groupId":"wg_95f6fdbfaa17593e"}}'
TS=$(date +%s)
SIG=$(printf '%s' "$TS.$BODY" | openssl dgst -sha256 -hmac "$CHAT_SERVICE_KEY" | awk '{print $2}')
curl -X POST "$CMS/webhook/web/designer" -H "Content-Type: application/json" \
  -H "X-Abc-Timestamp: $TS" -H "X-Abc-Signature: $SIG" -d "$BODY"
```

**Response** `200`

```json
{ "ok": true, "toast": { "type": "success", "text": "Đã gửi yêu cầu WEB-20261118-0006" }, "closeForm": true }
```

Các trường `values`: xem `docs/CHAT_SERVICE_UI_API.md` A12.

### 6.3 `card.action`

**Mô tả.** Người dùng bấm nút trên card. Chat phải kiểm tra trước: action có trong card và `actor.id ∈ visibleTo` của nút (nếu không → trả `403` cho UI, không gửi event). Đồng bộ, **không tự retry**. Danh sách action / `values`: `docs/CHAT_SERVICE_UI_API.md` 3.2.

**Request (body)**

```json
{
  "schema": "abc.event/v1", "eventId": "evt_cd3153cb-af22-43e5-b6f3-07512a4f8475", "type": "card.action",
  "botId": "b_1", "createdAt": "2026-11-18T04:39:34.715Z",
  "messageId": "m_146", "action": "accept_task",
  "values": { "normItemId": "996802", "quantity": 2 },
  "actor": { "id": "u_68", "openId": "local:c6816fc4-f523-46b3-8893-bd9b43e6abbc", "cmsUserId": 68, "name": "Trần Designer" },
  "group": { "id": "wg_95f6fdbfaa17593e" },
  "cardVersion": 1
}
```

**CURL**: như 6.2 với `BODY` ở trên.

**Response** `200` — thành công

```json
{
  "ok": true,
  "toast": { "type": "success", "text": "Đã nhận task" },
  "closeForm": true,
  "card": { "schema": "abc.card/v1", "header": { "code": "WEB-20261118-0006", "kind": "DESIGN", "title": "Banner Tết 2027",
            "status": { "text": "Đang làm", "tone": "info" } }, "visibleTo": null, "elements": [ "…" ] },
  "cardVersion": 2
}
```

**Response** `200` — CMS từ chối

```json
{ "ok": true, "toast": { "type": "error", "text": "Bạn không có quyền thao tác này" }, "closeForm": false }
```

Chat thay card của `messageId` khi `cardVersion` > phiên bản đang lưu, rồi phát realtime cho thành viên nhóm (lọc nút theo người xem).

### 6.4 `message.created`

**Mô tả.** Tin nhắn thường trong nhóm / thread. Bất đồng bộ — Chat **nên retry có backoff** khi CMS lỗi mạng / 5xx (CMS chống trùng theo `eventId`). CMS hiện chỉ ghi nhật ký.

**Request (body)**

```json
{
  "schema": "abc.event/v1", "eventId": "evt_1f9600f0-c6f6-4a46-bf8f-be4fa6f6e00c", "type": "message.created",
  "botId": "b_1", "createdAt": "2026-11-18T04:39:34.710Z",
  "message": { "id": "m_149", "text": "Nhờ team làm gấp giúp mình nhé", "threadRootId": "m_146" },
  "group": { "id": "wg_95f6fdbfaa17593e" },
  "sender": { "id": "u_67" }
}
```

**Response** `200`

```json
{ "ok": true }
```

### 6.5 `user.updated`

**Mô tả.** (Tuỳ chọn) Chat báo hồ sơ / vai trò hiển thị thay đổi. Bất đồng bộ, CMS chỉ ghi nhật ký. Body: `{ …khung chung, "user": { "id": "u_68", "...": "…" } }` → `{ "ok": true }`.

### 6.6 Lỗi xác thực webhook

```json
{ "error": "missing signature headers" }      // 401
{ "error": "invalid signature" }              // 401
{ "error": "timestamp out of range" }         // 401
{ "error": "Unknown webhook path" }           // 404 — URL không khớp webhookPath của bot nào
```

---

## 7. Bot API Chat Service phải có (CMS gọi)

Base: `CHAT_API_BASE` (dev `http://localhost:4010/bot/v1`). Mọi request có header `X-Chat-Service-Key: <CHAT_SERVICE_KEY>` ([3.4](#34-bot-api-cms--chat)). Không bọc envelope. Lỗi dạng `{ "code", "message", "retryable" }`. Mọi request ghi có header `Idempotency-Key` (UUID) — Chat phải trả lại kết quả cũ nếu gặp lại key (CMS retry một lần với **cùng key** khi lỗi `retryable` / 5xx / mạng).

Các request dưới là request thật CMS đã gửi.

### 7.1 Resolve user — `POST /users/resolve`

**Mô tả.** Đổi `openId` (CMS) → `actorId` (Chat). Người chưa từng đăng nhập Chat → Chat tạo actor (lấy hồ sơ qua `GET /workspace/users?openIds=`). CMS cache kết quả.

```bash
curl -X POST "$CHAT/bot/v1/users/resolve" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H "Content-Type: application/json" \
  -d '{"items":[{"openId":"local:c6816fc4-f523-46b3-8893-bd9b43e6abbc","name":"Trần Designer"}]}'
```

```json
{ "items": [ { "openId": "local:c6816fc4-f523-46b3-8893-bd9b43e6abbc", "actorId": "u_68", "name": "Trần Designer", "presence": "on" } ] }
```

### 7.2 Gửi tin / card — `POST /messages`

```bash
curl -X POST "$CHAT/bot/v1/messages" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"groupId":"wg_95f6fdbfaa17593e","bot":{"id":1,"name":"Demo Designer Bot"},"card":{"schema":"abc.card/v1","header":{"code":"WEB-20261118-0006","kind":"DESIGN","title":"Banner Tết 2027","status":{"text":"Chưa nhận","tone":"warning"}},"visibleTo":null,"elements":[…]}}'
```

```json
{ "messageId": "m_146", "cardVersion": 1 }
```

Body có thể là `{groupId, text}`, `{groupId, card}` hoặc kèm `threadRootId`. `bot: {id, name}` = bot CMS sở hữu nhóm — Chat dùng làm người gửi (vd. actor `b_<id>` tên `name`); thiếu thì dùng bot mặc định. Nhóm không tồn tại → `404 {"code":"not_found","message":"group","retryable":false}`.

### 7.3 Sửa card — `PATCH /messages/:id`

**Mô tả.** Thay card bằng phiên bản mới. Chỉ áp dụng khi `cardVersion` > phiên bản đang lưu; nhỏ hơn / bằng → bỏ qua (`ignored: true`). Sau khi thay, phát realtime cho nhóm.

```bash
curl -X PATCH "$CHAT/bot/v1/messages/m_146" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" -d '{"card":{…},"cardVersion":3}'
```

```json
{ "ok": true, "cardVersion": 3 }
```

### 7.4 Trả lời thread — `POST /messages/:id/thread`

```bash
curl -X POST "$CHAT/bot/v1/messages/m_146/thread" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" -d '{"text":"Đã giao Trần Designer · Chờ nhận task.","bot":{"id":1,"name":"Demo Designer Bot"}}'
```

```json
{ "messageId": "m_147" }
```

Các dòng CMS gửi: "Đã giao …", "… đã nhận task · …", "… đã nộp bài · Chờ duyệt", "Phiếu sửa #1 · …", "Tự giao … (hết xếp hàng)", "Gọi hỗ trợ · …", "… đã huỷ yêu cầu"…

### 7.5 Thông báo — `POST /notifications`

```bash
curl -X POST "$CHAT/bot/v1/notifications" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"actorId":"u_68","text":"Bạn có task mới: WEB-20261118-0006 · Banner Tết 2027","messageId":"m_146"}'
```

```json
{ "id": "n_148", "actorId": "u_68", "text": "Bạn có task mới: WEB-20261118-0006 · Banner Tết 2027", "messageId": "m_146", "createdAt": 1794976774706, "read": false }
```

### 7.6 Đẩy nhóm + thành viên — `PUT /groups/:id`

**Mô tả.** Tạo nếu chưa có, cập nhật tên và **thay toàn bộ** danh sách thành viên (theo `openId`, resolve sang actor). Phát `reload` cho người liên quan.

```bash
curl -X PUT "$CHAT/bot/v1/groups/wg_95f6fdbfaa17593e" -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"name":"Nhóm Thiết kế Web","members":[{"openId":"local:7a8060c4-…","cmsUserId":69,"name":"Lê Trợ Lý"},{"openId":"local:4d94607a-…","cmsUserId":67,"name":"Nguyễn Order"},{"openId":"local:c6816fc4-…","cmsUserId":68,"name":"Trần Designer"}]}'
```

```json
{ "id": "wg_95f6fdbfaa17593e", "memberCount": 3 }
```

### 7.7 API phụ (chưa dùng / tuỳ chọn)

| API | Body → Response | Trạng thái |
|---|---|---|
| `POST /messages/:id/pin` | `{}` → `{ "ok": true }` | CMS có hàm, chưa gọi |
| `PUT /presence` | `{ items: [{ actorId, status: "on"|"off", reason }] }` → `{ "updated": n }` | CMS có hàm, chưa gọi (dự kiến đồng bộ lịch nghỉ / ca) |
| `GET /groups/:id/members` | → `{ items: [{ id, name, title, presence, avatarUrl, role }] }` | có trong mock |
| `GET /groups`, `GET /groups/:id`, `GET /users/:id`, `POST /users/batch` | tra cứu | có trong mock, CMS chưa dùng |
| `GET /attachments/:id` | tệp đính kèm | chưa hỗ trợ |

---

## 8. Dữ liệu CMS đang cung cấp

### 8.1 Định danh

| Khái niệm | Chủ sở hữu | Dạng | Ghi chú |
|---|---|---|---|
| `cmsUserId` | CMS | số | `users.id` |
| `openId` | CMS | `ou_…` (user gắn thành viên Lark) / `local:<uuid>` (user tạo tay) | duy nhất, cầu nối CMS ↔ Chat |
| `actorId` | Chat | tuỳ Chat (mock: `u_<cmsUserId>`) | CMS lấy qua `users/resolve` |
| `chatId` / `groupId` | CMS | `wg_<hex>` | id nhóm Web, Chat dùng làm id nhóm |
| `messageId` | Chat | tuỳ Chat | CMS lưu để tìm đơn theo card |
| mã đơn | CMS | `WEB-YYYYMMDD-NNNN` | duy nhất toàn hệ thống |

### 8.2 User (`GET /users`, `auth/me`)

`cmsUserId`, `openId`, `username`, `email`, `name`, `avatarUrl`, `status` (`active`/`inactive`), `role` (`user`/`admin` — vai trò CMS), `isDesigner`, `webTeams[]`, `assistantOf[]` (chatId), `botAdminOf[]` (botId).

### 8.3 Nhóm Web (`GET /groups`)

`chatId`, `name`, `botId`, `timezone`, `isOrderHub`, `orderTeamCodes`, `teams[] {id, code, name}`, `assistants[]`, `members[] {cmsUserId, openId, name, avatarUrl, sources[], teamCodes[]}`.

### 8.4 Tổ, định mức, gợi ý người nhận, đơn

Qua access token (`docs/WEB_CHAT_UI_API.md`): tổ của nhóm (2.1), gợi ý người nhận kèm trạng thái rảnh / bận / ngoài ca / nghỉ (2.2), "Chờ tôi nhận" (3.1), danh sách / chi tiết đơn có lịch sử (4.1, 4.2), định mức (phụ lục).

### 8.5 Card `abc.card/v1`

CMS dựng toàn bộ card (header, trường, ghi chú, nút + form + xác nhận). Nút có `visibleTo` = danh sách `actorId` được thấy: Chat **phải lọc nút theo người xem** trước khi gửi xuống UI và **chặn action không có trong `visibleTo`**. Cấu trúc chi tiết: `docs/CHAT_SERVICE_UI_API.md` mục 2.

### 8.6 Trạng thái đơn

`queued` · `pending_accept` · `in_progress` · `paused` · `held` · `submitted` · `revising` · `done` · `cancelled` · `reference` (ý nghĩa: `docs/WEB_CHAT_UI_API.md` (Trạng thái đơn)).

---

## 9. Client API Chat Service cung cấp cho UI

Chat Service phải cung cấp cho UI (hợp đồng chi tiết + response mẫu: `docs/CHAT_SERVICE_UI_API.md`). Response bọc `{statusCode, code, message, element}`.

| API | Ghi chú triển khai |
|---|---|
| *(mọi API dưới)* | `Authorization: Bearer <access token CMS>` — kiểm bằng JWKS của CMS ([3.3](#33-access-token-do-cms-phát-ui-dùng-cho-cả-cms-và-chat)); không có login / logout / phát token riêng |
| `GET /api/client/v1/chat/auth/me` | actor của người dùng trong chat (tạo từ hồ sơ CMS lần đầu) |
| `GET /api/client/v1/chat/bootstrap` | chỉ nhóm user là thành viên; nút card đã lọc theo người xem |
| `GET /api/client/v1/chat/stream` (SSE) hoặc WebSocket | gói `message`, `read`, `notification`, `presence`, `user`, `reload` |
| `POST /api/client/v1/chat/groups/refresh` | gọi lại CMS `GET /groups` ngay |
| `POST /api/client/v1/chat/groups/:id/messages` | lưu + phát + webhook `message.created` |
| `POST /api/client/v1/chat/groups/:id/read` | trạng thái đọc |
| `POST /api/client/v1/chat/messages/:id/actions` | kiểm `visibleTo` → webhook `card.action` → áp card mới |
| `POST /api/client/v1/chat/forms/:formId/submit` | webhook `form.submit` |
| `POST /api/client/v1/chat/notifications/read` | |

---

## 10. Checklist triển khai

**Chat → CMS**

- [ ] Không làm đăng nhập: kiểm access token của CMS (JWKS `<CMS>/api/client/v1/workspace/.well-known/jwks.json`, `iss=designer-cms`, `aud=abc-workspace`, `typ=access`), actor = `cmsUserId`.
- [ ] Đồng bộ nhóm: poll `GET /workspace/groups` + nhận `PUT /bot/v1/groups/:id`; `memberIds` = đúng danh sách CMS.
- [ ] Hồ sơ user qua `GET /workspace/users` (ghép `avatarUrl` tương đối với base CMS).
- [ ] Không đọc database CMS.

**Webhook**

- [ ] Ký HMAC bằng `CHAT_SERVICE_KEY` trên raw body, `X-Abc-Timestamp` (giây), `eventId` duy nhất.
- [ ] `form.submit` / `card.action`: đồng bộ, timeout ~5 giây, không retry, chuyển `toast` / `closeForm` cho UI, áp `card` theo `cardVersion`.
- [ ] `message.created` / `user.updated`: bất đồng bộ, retry có backoff.
- [ ] Event có `actor.cmsUserId` / `actor.openId`.

**Bot API**

- [ ] Kiểm `X-Chat-Service-Key` trên mọi request Bot API (không có `auth/token`).
- [ ] `users/resolve`, `messages` (người gửi theo `bot`), `PATCH messages/:id` (theo `cardVersion`), `messages/:id/thread`, `notifications`, `PUT groups/:id`.
- [ ] `Idempotency-Key`: trả lại kết quả cũ khi lặp key.
- [ ] Lỗi dạng `{code, message, retryable}`.

**JWT / bảo mật**

- [ ] Cache JWKS của CMS (vài giờ), tải lại khi gặp `kid` lạ; trả `401` khi token sai / hết hạn (UI tự làm mới ở CMS).
- [ ] Lọc nút card theo `visibleTo` và chặn action ngoài `visibleTo`.
- [ ] Chỉ thành viên nhóm mới xem / gửi tin / bấm nút trong nhóm.
