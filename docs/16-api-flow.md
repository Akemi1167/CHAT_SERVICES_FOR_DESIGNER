# 16 — Luồng gọi API theo thứ tự (bắt đầu từ FE)

Mỗi mục là một kịch bản: **ai gọi gì, theo thứ tự nào, nhận lại gì, và event realtime nào bắn ra**.
Ký hiệu: **FE** = web chat · **CHAT** = service này · **CMS** = Task Engine.

---

## A. Mở app (lần đầu / sau khi mất phiên)

```
FE ──1──► CMS   POST /api/client/v1/workspace/auth/login {identifier, password}
   ◄─────       200 {accessToken, refreshToken, user}

FE ──2──► CHAT  GET /api/client/v1/chat/bootstrap          Bearer <accessToken>
   ◄─────       {me, groups[], notifications[], unreadNotifications}
                  (lần đầu CHAT tự: kiểm token bằng JWKS của CMS → tạo actor
                   → GET /workspace/users?ids= lấy hồ sơ)

FE ──3──► CHAT  WebSocket  ws(s)://…/ws?token=<accessToken>
   ◄─────       frame {"type":"ready","payload":{user_id, display_name, role, conn_id}}
```

| # | Gọi | Dùng để |
|---|---|---|
| 1 | CMS `auth/login` | lấy token (Chat **không** có login) |
| 2 | CHAT `bootstrap` | danh sách nhóm + `unreadCount` + thông báo → vẽ sidebar |
| 3 | CHAT `/ws` | mở realtime, giữ suốt phiên |

Token hết hạn (401 ở bất kỳ request nào) → FE gọi CMS `POST /workspace/auth/refresh`, rồi **mở lại WebSocket** bằng token mới.

---

## B. Mở một nhóm

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

## C. Gửi tin nhắn thường

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
> broadcast (xem [11-frontend.md](11-frontend.md) §5).

---

## D. Trả lời & thread

```
FE ──1──► CHAT  WS message.send  payload {conversation_id, content,
                                          reply_to_id: <tin trích dẫn>,
                                          thread_id:  <tin mở thread>}
FE ──2──► CHAT  GET /api/client/v1/chat/messages/{id}/replies?mode=thread   (mở panel thread)
   ◄─────       {root, replies[] (cũ→mới), replyCount, threadCount}
FE ──3──► CHAT  GET …/replies?mode=direct                                  (xem ai trả lời 1 bong bóng)
```

---

## E. Tạo yêu cầu (đơn) — vòng tròn đầy đủ

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

---

## F. Bấm nút trên card

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

## G. CMS chủ động (tự giao, đổi định mức, nhắc việc)

Không có FE trong luồng này — FE chỉ nhận realtime:

```
CMS ──► CHAT  PATCH /bot/v1/messages/{id} {card, cardVersion:3}   → {ok, cardVersion}
                 (cardVersion ≤ bản đang lưu → {ok, ignored:true}, không đổi gì)
CMS ──► CHAT  POST  /bot/v1/messages/{id}/thread {text:"Tự giao … · Chờ nhận task."}
CMS ──► CHAT  POST  /bot/v1/notifications {actorId, text, messageId}
        CHAT ══► realtime "message" + "notification"
```

---

## H. Thành viên nhóm thay đổi (thêm tay / link mời / đổi tổ)

```
CMS ──► CHAT  PUT /bot/v1/groups/{chatId} {name, members[]}   → {id, memberCount}
        CHAT đặt danh sách thành viên = ĐÚNG danh sách CMS gửi (ai mất khỏi list → mất quyền xem)
        CHAT ══► realtime "reload" cho người vừa vào + người vừa bị gỡ
FE  ──► CHAT  GET /api/client/v1/chat/bootstrap         (nhận "reload" thì tải lại danh sách nhóm)
```

Vào nhóm bằng link mời: FE gọi CMS `POST /workspace/groups/join {token}` trước, rồi
`POST /api/client/v1/chat/groups/refresh` để Chat hỏi lại CMS ngay (không phải chờ chu kỳ 60s).

---

## I. Các luồng phụ

| Việc | FE gọi | Realtime bắn ra |
|---|---|---|
| Đang nhập | WS `{"type":"typing","payload":{conversation_id, typing:true}}` | `typing` (không gửi lại cho chính mình) |
| Online/OFF | tự động khi mở/đóng WS; CMS đẩy lịch OFF qua `PUT /bot/v1/presence` | `presence` cho người **chung nhóm** |
| Ghim tin | `POST /api/v1/messages/{id}/pin` (hoặc CMS `POST /bot/v1/messages/{id}/pin`) | `message` (tin đã cập nhật) |
| Xem tệp | `GET /api/v1/messages/{mid}/attachments/{aid}` (`?redirect=true` → 302) | — |
| Thông báo | `GET …/chat/notifications`, `POST …/chat/notifications/read` | `notification` |

---

## J. Bảng tra nhanh — FE gọi gì, khi nào

| Thời điểm | Request |
|---|---|
| Mở app | CMS `auth/login` → CHAT `GET /chat/bootstrap` → mở `/ws` |
| Mở nhóm | `GET /chat/groups/{id}/messages` → `POST /chat/groups/{id}/read` |
| Cuộn lên | `GET …/messages?cursor=<nextCursor>` |
| Gửi tin | WS `message.send` (fallback `POST /chat/groups/{id}/messages`) |
| Mở thread | `GET /chat/messages/{id}/replies?mode=thread` |
| Tạo yêu cầu | `POST /chat/forms/create_task/submit` |
| Bấm nút card | `POST /chat/messages/{id}/actions` |
| Nhận `reload` | `GET /chat/bootstrap` lại |
| 401 | CMS `auth/refresh` → mở lại `/ws` |

---

## K. Tự test luồng ngay trên máy

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

Giao diện thử tay: mở `chat-thread-demo.html`. Realtime bằng dòng lệnh: `make ws TOKEN=$TOK`.

---

Danh sách endpoint đầy đủ: [05-api.md](05-api.md) · chi tiết cho FE: [11-frontend.md](11-frontend.md) ·
hợp đồng với CMS: [10-cms-integration.md](10-cms-integration.md).
