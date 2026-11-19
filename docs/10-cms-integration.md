# 10 — Tích hợp CMS (Task Engine)

Bản này map tài liệu *"Tích hợp Chat Service (Go) ↔ CMS"* vào code thật trong repo.

## 1. Phân vai

| Nguồn sự thật | Thuộc về | Trong code |
|---|---|---|
| Tài khoản, phiên đăng nhập, quyền | **CMS** | Chat chỉ verify token bằng JWKS, không có login/logout |
| Nhóm Web, thành viên nhóm | **CMS** | `internal/service/cmssync` — Chat không tự tạo nhóm |
| Đơn / task / card (`abc.card/v1`) | **CMS** | Chat chỉ lưu `messages.card` + `card_version`, forward event |
| actorId, tin nhắn, cardVersion đang lưu, trạng thái đọc, presence | **Chat** | `users.actor_id`, `messages`, `conversation_members.last_read_at` |

Chat **không đọc database CMS**: mọi dữ liệu đi qua Workspace API.

## 2. Bốn luồng và chỗ cài đặt

| Luồng | Hướng | Xác thực | Code |
|---|---|---|---|
| (1) UI ↔ Chat | REST + WebSocket | access token CMS (JWKS) | `internal/transport/httpserver/clientapi`, `.../ws` |
| (2) Chat → CMS | webhook | HMAC `X-Abc-Signature` | `internal/cms/client.go` (`SendEvent`, `Sign`) |
| (3) CMS → Chat | Bot API | `X-Chat-Service-Key` | `internal/transport/httpserver/botapi` |
| (5) Chat → CMS | Workspace API | `X-Chat-Service-Key` | `internal/cms/client.go` (`Groups`, `Users`) |

## 3. Cấu hình

```bash
CMS_ENABLED=true
CHAT_SERVICE_KEY=mock_service_key                 # khoá dùng chung cho cả 3 hướng
CMS_BASE_URL=http://localhost:3001                # để ghép avatarUrl tương đối
CMS_WORKSPACE_BASE_URL=http://localhost:3001/api/client/v1/workspace
CMS_WEBHOOK_URL=http://localhost:3001/webhook/web/designer
CMS_JWT_ISSUER=designer-cms
CMS_JWT_AUDIENCE=abc-workspace
CMS_JWKS_URL=                                     # trống = <workspace>/.well-known/jwks.json
CMS_JWKS_CACHE_TTL=6h
CMS_SYNC_INTERVAL=60s                             # poll GET /workspace/groups
CMS_WEBHOOK_TIMEOUT=5s                            # form.submit / card.action (đồng bộ)
CMS_OUTBOX_INTERVAL=10s                           # gửi lại message.created
AUTH_ALLOW_DEV_TOKEN=true                         # vẫn nhận token HS256 của `make token` (tắt ở prod)
TASK_ENGINE_ENABLED=false                         # module task nội bộ của Chat (mặc định TẮT)
```

Bật `CMS_ENABLED=true` thì `JWT_SECRET` không còn bắt buộc (trừ khi vẫn cho token dev).

## 4. Xác thực token của CMS

`internal/pkg/jwtauth/jwks.go`: tải JWKS, cache theo `CMS_JWKS_CACHE_TTL`, gặp `kid` lạ thì tải lại
ngay; chỉ nhận **RS256**, kiểm `iss` / `aud` / `typ=access` / `exp`. Token sai → `401`, UI tự làm mới
ở CMS rồi gọi lại.

`ChainVerifier` cho phép chạy song song token CMS (RS256) và token dev (HS256) — tiện cho Postman
và `make ws` trong lúc phát triển, tắt bằng `AUTH_ALLOW_DEV_TOKEN=false`.

Lần đầu gặp `cmsUserId`: Chat tạo user local, sinh `actor_id = u_<cmsUserId>`, rồi gọi
`GET /workspace/users?ids=` lấy hồ sơ (tên, avatar, tổ, trợ lý, admin bot) và **lưu lại**
(`users.cms_profile`) để FE/`visibleTo` dùng mà không phải hỏi CMS mỗi lần.

## 5. Đồng bộ nhóm & thành viên

- Poll `GET /workspace/groups` lúc khởi động + mỗi `CMS_SYNC_INTERVAL`.
- CMS đẩy ngay qua `PUT /bot/v1/groups/:chatId`.

**Khi nào Chat bắn event `reload`**: chỉ khi có thay đổi thật, so sánh danh sách thành viên
trước/sau (`cmssync.diff`) và tên/`cms_info` (`UpsertFromCMS` trả thêm `changed`):

| Thay đổi | `reason` | Gửi cho ai |
|---|---|---|
| thêm / bớt thành viên | `group.members_changed` | thành viên hiện tại **∪** người vừa bị gỡ |
| đổi tên nhóm hoặc `cms_info` | `group.updated` | thành viên hiện tại |
| không đổi gì | *(không bắn)* | — |

`synced_at` đổi mỗi chu kỳ đồng bộ nên **không** tính là thay đổi — nếu tính thì vòng poll
60 giây sẽ bắn `reload` cho cả nhóm mãi mãi dù chẳng có gì mới.
- `conversations.external_id` = `chatId` (`wg_…`), `source = cms`; mọi API nhận **cả `wg_…` lẫn UUID nội bộ**.
- `ReplaceMembers` đặt danh sách thành viên **đúng bằng** danh sách CMS trả: ai không còn trong danh
  sách bị `left_at` → mất quyền xem nhóm ngay (đã kiểm chứng: trả `403 NOT_CONVERSATION_MEMBER`).
- `sources` / `teamCodes` lưu vào `conversation_members`; `assistant` được map thành role `admin`
  trong Chat (chỉ để kiểm quyền nội bộ — quyền nghiệp vụ vẫn do CMS quyết).
- Người vừa vào/vừa bị gỡ nhận event realtime `reload`.

## 6. Card: lọc `visibleTo` và chặn action

`internal/service/card/card.go` (có unit test):

- `FilterForViewer(card, actorId)` — duyệt sâu, bỏ mọi node có `visibleTo` không chứa người xem;
  `visibleTo` vắng/null/rỗng = ai cũng thấy; card bị giới hạn ở cấp cao nhất thì **không trả** cho người ngoài.
- `CanAct(card, action, actorId)` — chặn ngay tại Chat trước khi gửi event (trả `403`, không làm phiền CMS).
- Realtime: mỗi người nhận **bản card đã lọc riêng** (không broadcast chung một payload).

## 7. Event Chat → CMS

| Event | Kiểu | Cài đặt |
|---|---|---|
| `form.submit` | đồng bộ, timeout `CMS_WEBHOOK_TIMEOUT`, không retry | `card.Service.SubmitForm` |
| *(khung chung)* | `schema`, `eventId` (UUID), `type`, `createdAt`, `botId` (`b_<botId của nhóm>`), `actor{id,openId,cmsUserId,name}` | `cms.BuildEvent` |
| `card.action` | đồng bộ; áp card mới nếu `cardVersion` lớn hơn rồi phát realtime | `card.Service.RunAction` |
| `message.created` | **bất đồng bộ**, ghi `webhook_outbox`, worker retry backoff 2^n (tối đa 10 phút) | `internal/service/outbox` |
| `user.updated` | bất đồng bộ (dùng chung outbox) | — |

Chữ ký: `hex(HMAC_SHA256(CHAT_SERVICE_KEY, "<timestamp>.<rawBody>"))` tính trên **đúng bytes đã gửi**.
`eventId` duy nhất mỗi event; gửi lại giữ nguyên `eventId` để CMS chống trùng.

## 8. Bot API (CMS gọi Chat)

`POST /bot/v1/users/resolve` · `POST /bot/v1/messages` · `PATCH /bot/v1/messages/:id` ·
`POST /bot/v1/messages/:id/thread` · `POST /bot/v1/messages/:id/pin` · `POST /bot/v1/notifications` ·
`PUT /bot/v1/presence` · `PUT /bot/v1/groups/:id` · `GET /bot/v1/groups/:id/members` ·
`GET /bot/v1/attachments/:id` · `GET /bot/v1/groups?q=&source=` · `GET /bot/v1/groups/:id` ·
`GET /bot/v1/users/:id` · `POST /bot/v1/users/batch`

Realtime cho FE: `GET /api/client/v1/chat/ws?token=<access token CMS>` — đúng mục 9 của spec ("SSE hoặc WebSocket"), payload cùng hình dạng với REST của bề mặt này.

→ **Đủ toàn bộ mục 7 của `CHAT_SERVICE_INTEGRATION.md`, kể cả 7.7 (phần CMS "chưa dùng").**
`GET /users/:id` nhận cả `actorId` (`u_6`), `openId` hay UUID nội bộ; `POST /users/batch` nhận
`{actorIds[], openIds[], cmsUserIds[]}`.

Ba endpoint tài liệu ghi "chưa dùng / tuỳ chọn" nay đã có:

| Endpoint | Hành vi |
|---|---|
| `POST /messages/:id/pin` | `{}` → `{ok, messageId, pinned}`; `{"pinned":false}` để bỏ ghim; ghim lại tin đã ghim không đổi gì. Cả nhóm nhận event `message` để cập nhật. Danh sách ghim: `GET /api/v1/conversations/:id/pinned`. |
| `PUT /presence` | `{items:[{actorId,status,reason}]}` → `{updated:n}`. `on` → online; `off` **kèm `reason`** → `off_duty` (chữ OFF cạnh tên); `off` không reason → offline. Actor lạ bị bỏ qua, không lỗi. Chat phát event `presence` cho **những người chung hội thoại**. |
| `GET /attachments/:id` | Chat **không lưu file**, chỉ lưu URL do service upload/CDN cấp → trả metadata (`url`, `name`, `mimeType`, `sizeBytes`, kích thước, `messageId`, `groupId`); thêm `?redirect=true` để 302 thẳng tới URL. |

- Kiểm `X-Chat-Service-Key` **constant-time**; sai → `401 {"code":"invalid_service_key",...,"retryable":false}`.
- Mọi request ghi nhận `Idempotency-Key`: gặp lại key trả **nguyên kết quả cũ** (bảng `idempotency_keys`).
- `PATCH` chỉ áp dụng khi `cardVersion` **lớn hơn** bản đang lưu, nhỏ hơn/bằng → `{"ok":true,"ignored":true}`.
- Lỗi: `{code, message, retryable}` — `retryable=true` khi lỗi hệ thống (5xx).

## 9. API cho FE

`/api/client/v1/chat/*`, envelope `{statusCode, code, message, element}`:

`GET auth/me` · `GET bootstrap` · `POST groups/refresh` · `GET|POST groups/:id/messages` ·
`POST groups/:id/read` · `POST messages/:id/actions` · `POST forms/:formId/submit` ·
`GET notifications` · `POST notifications/read`

`:id` nhận cả `wg_…` và UUID. Realtime vẫn là WebSocket `/ws?token=<access token>` với tên event theo
hợp đồng: `message` · `read` · `notification` · `presence` · `user` · `reload` (`typing`,
`message.deleted` là mở rộng của Chat).

FE gửi `team_id` trong `values` của `POST forms/create_task/submit` — Chat **không diễn giải**, chuyển
nguyên sang CMS trong `form.submit.values`.

Bộ API `/api/v1/*` (envelope `{data, meta}`) vẫn giữ cho Postman/CLI/dịch vụ nội bộ.

## 10. Module task nội bộ

`TASK_ENGINE_ENABLED=false` (mặc định): route `/api/v1/tasks`, `/teams`, `/revisions` **không** được
đăng ký, vì CMS là nguồn sự thật của đơn. Code + migration vẫn còn trong repo
(`internal/service/task`, `migrations/000003_task.*`) để dùng khi chạy độc lập, bật lại bằng env.

## 11. Đã kiểm chứng (CMS giả lập theo đúng tài liệu)

| Kịch bản | Kết quả |
|---|---|
| Đăng nhập bằng token RS256 của CMS → bootstrap | actor `u_68`, role `designer`, avatar ghép base URL |
| Poll `GET /workspace/groups` | nhóm `wg_…` + 2 thành viên kèm `sources`/`teamCodes` |
| `POST /bot/v1/messages` (card) | `{"messageId","cardVersion":1}` |
| Gửi lại cùng `Idempotency-Key` | trả **y hệt**, không tạo tin mới |
| Sai service key | `401 invalid_service_key` |
| Lọc `visibleTo` | `u_68` thấy nút "Nhận task", `u_67` thấy "Huỷ" |
| `u_67` bấm `accept_task` | `403 ACTION_NOT_ALLOWED` (chặn tại Chat, không gửi CMS) |
| `u_68` bấm `accept_task` | webhook ký HMAC hợp lệ → CMS trả card v2 → Chat áp dụng, cả nhóm thấy `cardVersion: 2` |
| `POST forms/create_task/submit` (có `teamId`) | toast "Đã gửi yêu cầu WEB-…" |
| `POST /bot/v1/notifications` | user nhận realtime `notification`, API đếm `unread` |
| Gửi tin nhắn thường | CMS nhận `message.created` qua outbox |
| `PUT /bot/v1/groups` bỏ `u_67` | `u_67` mất quyền (`403`), `u_68` vẫn xem được; `groups/refresh` trả lại đúng danh sách CMS |
| Realtime | `u_68` nhận `message` với nút "Nhận task", `u_67` nhận cùng tin nhưng nút "Huỷ" |

## 12. Còn thiếu / cần CMS thật để chốt

- Chưa test với CMS dev thật (đang chờ URL + `CHAT_SERVICE_KEY`); CMS giả lập dựng theo đúng payload
  trong tài liệu.
- `user.updated` đã có đường outbox nhưng chưa có chỗ phát (cần chốt khi nào Chat coi là hồ sơ đổi).
- SSE `GET /chat/stream`: đang dùng WebSocket (tài liệu cho phép chọn một trong hai).
