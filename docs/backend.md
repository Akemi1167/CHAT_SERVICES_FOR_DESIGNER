# Backend — API, tích hợp CMS, test

> Mới vào dự án? Đọc [FLOW.md](FLOW.md) trước — sơ đồ ba bên + 9 luồng kèm lệnh chạy thử.

Hợp đồng API của service, cách tích hợp CMS và cách thử.

Sửa code → [source.md](source.md) · Làm FE → [frontend.md](frontend.md) ·
Spec gốc do CMS đưa → [CHAT_SERVICE_INTEGRATION.md](CHAT_SERVICE_INTEGRATION.md)

## Nội dung

- [Danh sách API](#danh-sách-api)
- [Tích hợp CMS (Designer Bot)](#tích-hợp-cms-designer-bot)
- [Bot @bot và mẫu (template)](#bot-bot-và-mẫu-template)
- [Module task nội bộ](#module-task-nội-bộ)
- [Postman](#postman)
- [Hướng dẫn test](#hướng-dẫn-test)

---

## Danh sách API

Tất cả REST endpoint (trừ health) yêu cầu `Authorization: Bearer <JWT>`.
Response có dạng `{"data": ..., "meta": {...}}` hoặc `{"error":{"code","message","details"}}`.
`meta` của endpoint danh sách là `{"next_cursor":"...","has_more":true}`.

### REST

> `:id` của mọi route hội thoại nhận **cả hai dạng id của cùng một nhóm**: UUID nội bộ
> hoặc `chatId` của CMS (`wg_<hex>` = `conversations.external_id`). Nhờ vậy `/api/v1`,
> `/bot/v1` và `/api/client/v1` dùng chung một giá trị id.

| Method | Path | Mô tả |
|---|---|---|
| GET | `/healthz` | liveness |
| GET | `/readyz` | readiness (ping DB) |
| GET | `/ws?token=` | WebSocket, hình dạng nội bộ (snake_case) |
| GET | `/api/client/v1/chat/ws?token=` | WebSocket cho FE — **cùng access token CMS với REST**, field camelCase trùng REST (xem [Giao thức WebSocket](frontend.md#giao-thức-websocket)) |
| GET | `/api/v1/me` | thông tin user từ token |
| POST | `/api/v1/conversations` | tạo chat riêng (`type=direct`, 1 `member_user_ids`) hoặc nhóm (`type=group`, có `name`) |
| GET | `/api/v1/conversations?type=&source=&q=&cursor=&limit=` | danh sách hội thoại + `unread_count`. `type`=`direct\|group`, `source`=`cms\|local`, `q` tìm trong tên (không phân biệt hoa thường). Giá trị `type`/`source` lạ → 400 `invalid_type` / `invalid_source` |
| GET | `/api/v1/conversations/:id` | chi tiết + thành viên |
| POST | `/api/v1/conversations/:id/members` | thêm thành viên (owner/admin) |
| DELETE | `/api/v1/conversations/:id/members/:user_id` | xoá thành viên / tự rời nhóm |
| GET | `/api/v1/conversations/:id/messages?thread_id=&only_root=&cursor=&limit=` | timeline (mới nhất trước) |
| POST | `/api/v1/conversations/:id/messages` | gửi tin nhắn (kể cả `@bot ...`) |
| POST | `/api/v1/conversations/:id/read` | đánh dấu đã đọc, trả `unread_count` |
| GET | `/api/v1/messages/search?q=&conversation_id=&sender_id=&from_date=&to_date=` | tìm theo nội dung / mã task / người gửi / ngày |
| GET | `/api/v1/messages/:message_id/replies?mode=direct\|thread&cursor=&limit=` | **phản hồi của 1 tin nhắn**: `direct` = trả lời trực tiếp (`reply_to_id`), `thread` = cả thread tin đó mở (`thread_id`); trả `root`, `replies` (cũ→mới), `reply_count`, `thread_count` |
| GET | `/api/v1/messages/:message_id/readers` | danh sách người đã xem |
| POST | `/api/v1/messages/:message_id/pin` | ghim / bỏ ghim (`{"pinned":false}`) |
| GET | `/api/v1/conversations/:id/pinned?limit=50` | danh sách tin đã ghim (mới ghim trước) |
| GET | `/api/v1/messages/:message_id/attachments/:attachment_id?redirect=` | metadata tệp đính kèm, `redirect=true` → 302 tới URL |
| DELETE | `/api/v1/messages/:message_id` | xoá mềm |
| GET | `/api/v1/bots` | danh sách bot |
| GET | `/api/v1/bots/:slug/templates` | schema các mẫu (FE render form) |
| POST | `/api/v1/bots/:slug/submissions` | gửi mẫu từ form |
| GET | `/api/v1/bots/:slug/submissions?conversation_id=&status=` | các mẫu đã lưu |

#### Task (xem [Module task nội bộ](#module-task-nội-bộ))

| Method | Path | Mô tả |
|---|---|---|
| POST | `/api/v1/tasks` | tạo yêu cầu (bỏ trống `assignee_ids` → tự gợi ý người trong tổ) |
| GET | `/api/v1/tasks?conversation_id=&thread_id=&team_id=&assignee_id=&created_by=&status=&waiting_accept=&mine=&q=` | danh sách task; `waiting_accept=true` = **chờ nhận** |
| GET | `/api/v1/tasks/:id` | chi tiết (kèm card + phiếu sửa + tổ) |
| GET | `/api/v1/tasks/code/:code` | tra cứu theo mã `T-000001` |
| PATCH | `/api/v1/tasks/:id` | điều chỉnh tiêu đề/mô tả/tổ/số lượng/định mức/deadline/ưu tiên (admin hoặc người tạo) |
| POST | `/api/v1/tasks/:id/accept` | **nhận task** — body `{"quota":"A4","quantity":3}` (bắt buộc) |
| POST | `/api/v1/tasks/:id/pause` | tạm dừng |
| POST | `/api/v1/tasks/:id/resume` | tiếp tục |
| POST | `/api/v1/tasks/:id/hold` | treo — bắt buộc `{"note":"lý do"}` |
| POST | `/api/v1/tasks/:id/complete` | hoàn thành (bị chặn nếu còn phiếu sửa mở) |
| POST | `/api/v1/tasks/:id/assignees` | chia task / gọi hỗ trợ / gọi Edit — `{"user_ids":[...],"role":"primary|support|edit"}` |
| POST | `/api/v1/tasks/:id/reassign` | đổi người phụ trách — `{"to_user_id":"...","assignment_id":"..."}` |
| POST | `/api/v1/tasks/:id/cancel` | huỷ task |
| GET | `/api/v1/tasks/:id/events` | lịch sử task |
| GET | `/api/v1/tasks/:id/revisions` | danh sách phiếu sửa |
| POST | `/api/v1/tasks/:id/revisions` | tạo **phiếu sửa** — `{"reason","revision_type","quantity","assignment_id"}` |
| POST | `/api/v1/revisions/:revision_id/complete` | hoàn thành riêng 1 phiếu sửa |

Các thao tác trên card nhận thêm `assignment_id` (admin tác động card người khác) và `note`.

#### Tổ / bộ phận

| Method | Path | Mô tả |
|---|---|---|
| GET | `/api/v1/teams` | danh sách tổ (seed: `design`, `edit`, `order`) |
| POST | `/api/v1/teams` | tạo tổ — `{"code","name","kind"}` |
| GET | `/api/v1/teams/:id/members` | thành viên tổ |
| POST | `/api/v1/teams/:id/members` | thêm thành viên — `{"user_ids":[...],"role":"leader|assistant|member"}` |
| DELETE | `/api/v1/teams/:id/members/:user_id` | bỏ thành viên khỏi tổ |
| GET | `/api/v1/teams/:id/suggest` | **gợi ý người phụ trách** (không OFF + ít việc nhất trước) |

WebSocket: `GET /ws?token=<JWT>` — xem [Giao thức WebSocket](frontend.md#giao-thức-websocket).

### gRPC (`api/proto/chat/v1`)

Auth: metadata `authorization: Bearer <JWT>`. Lỗi trả về theo `codes` chuẩn
(`InvalidArgument`, `Unauthenticated`, `PermissionDenied`, `NotFound`, `AlreadyExists`, `Internal`).

`ChatService`: `CreateConversation`, `GetConversation`, `ListConversations`, `AddMembers`,
`RemoveMember`, `SendMessage`, `ListMessages`, `SearchMessages`, `MarkRead`.

Bot API (CMS gọi): thêm `POST /bot/v1/messages/:id/pin`, `PUT /bot/v1/presence`,
`GET /bot/v1/attachments/:id` — xem [Tích hợp CMS (Designer Bot)](#tích-hợp-cms-designer-bot) §8.

`BotService`: `ListBots`, `ListBotTemplates`, `SubmitBotTemplate`, `ListBotSubmissions`.

`TaskService`: `CreateTask`, `GetTask`, `ListTasks`, `UpdateTask`, `CancelTask`,
`AcceptTask`, `PauseTask`, `ResumeTask`, `HoldTask`, `CompleteTask` (5 RPC này dùng chung
`TaskActionRequest`), `AddAssignees`, `ReassignTask`, `CreateRevision`, `CompleteRevision`,
`ListRevisions`, `ListTaskEvents`, `ListTeams`, `CreateTeam`, `AddTeamMembers`,
`RemoveTeamMember`, `ListTeamMembers`, `SuggestAssignees`.

Lưu ý: các trường trạng thái/role của task dùng **string** (không dùng enum protobuf) để REST/WS/gRPC
dùng chung một bộ giá trị và thêm trạng thái mới không phá hợp đồng — giá trị hợp lệ ghi trong
`api/proto/chat/v1/task.proto`.

Thử nhanh:

```bash
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext -H "authorization: Bearer $TOKEN" \
  -d '{"page":{"limit":10}}' localhost:9090 chat.v1.ChatService/ListConversations
```

---

## Tích hợp CMS (Designer Bot)

> Đội **CMS** cần biết gọi API nào của Chat khi có thay đổi (tạo đơn, đổi trạng thái, sửa
> card, đổi thành viên nhóm) thì đọc [cms-goi-chat.md](cms-goi-chat.md) — tài liệu riêng cho
> chiều CMS → Chat, có body/response thật của từng endpoint.

Bản này map tài liệu *"Tích hợp Chat Service (Go) ↔ CMS"* vào code thật trong repo.

### 1. Phân vai

| Nguồn sự thật | Thuộc về | Trong code |
|---|---|---|
| Tài khoản, phiên đăng nhập, quyền | **CMS** | Chat chỉ verify token bằng JWKS, không có login/logout |
| Nhóm Web, thành viên nhóm | **CMS** | `internal/service/cmssync` — Chat không tự tạo nhóm |
| Đơn / task / card (`abc.card/v1`) | **CMS** | Chat chỉ lưu `messages.card` + `card_version`, forward event |
| actorId, tin nhắn, cardVersion đang lưu, trạng thái đọc, presence | **Chat** | `users.actor_id`, `messages`, `conversation_members.last_read_at` |

Chat **không đọc database CMS**: mọi dữ liệu đi qua Workspace API.

### 2. Bốn luồng và chỗ cài đặt

| Luồng | Hướng | Xác thực | Code |
|---|---|---|---|
| (1) UI ↔ Chat | REST + WebSocket | access token CMS (JWKS) | `internal/transport/httpserver/clientapi`, `.../ws` |
| (2) Chat → CMS | webhook | HMAC `X-Abc-Signature` | `internal/cms/client.go` (`SendEvent`, `Sign`) |
| (3) CMS → Chat | Bot API | `X-Chat-Service-Key` | `internal/transport/httpserver/botapi` |
| (5) Chat → CMS | Workspace API | `X-Chat-Service-Key` | `internal/cms/client.go` (`Groups`, `Users`) |

### 3. Cấu hình

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

### 4. Xác thực token của CMS

`internal/pkg/jwtauth/jwks.go`: tải JWKS, cache theo `CMS_JWKS_CACHE_TTL`, gặp `kid` lạ thì tải lại
ngay; chỉ nhận **RS256**, kiểm `iss` / `aud` / `typ=access` / `exp`. Token sai → `401`, UI tự làm mới
ở CMS rồi gọi lại.

`ChainVerifier` cho phép chạy song song token CMS (RS256) và token dev (HS256) — tiện cho Postman
và `make ws` trong lúc phát triển, tắt bằng `AUTH_ALLOW_DEV_TOKEN=false`.

Lần đầu gặp `cmsUserId`: Chat tạo user local, sinh `actor_id = u_<cmsUserId>`, rồi gọi
`GET /workspace/users?ids=` lấy hồ sơ (tên, avatar, tổ, trợ lý, admin bot) và **lưu lại**
(`users.cms_profile`) để FE/`visibleTo` dùng mà không phải hỏi CMS mỗi lần.

### 5. Đồng bộ nhóm & thành viên

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

### 6. Card: lọc `visibleTo` và chặn action

`internal/service/card/card.go` (có unit test):

- `FilterForViewer(card, actorId)` — duyệt sâu, bỏ mọi node có `visibleTo` không chứa người xem;
  `visibleTo` vắng/null/rỗng = ai cũng thấy; card bị giới hạn ở cấp cao nhất thì **không trả** cho người ngoài.
- `CanAct(card, action, actorId)` — chặn ngay tại Chat trước khi gửi event (trả `403`, không làm phiền CMS).
- Realtime: mỗi người nhận **bản card đã lọc riêng** (không broadcast chung một payload).

### 7. Event Chat → CMS

| Event | Kiểu | Cài đặt |
|---|---|---|
| `form.submit` | đồng bộ, timeout `CMS_WEBHOOK_TIMEOUT`, không retry | `card.Service.SubmitForm` |
| *(khung chung)* | `schema`, `eventId` (UUID), `type`, `createdAt`, `botId` (`b_<botId của nhóm>`), `actor{id,openId,cmsUserId,name}` | `cms.BuildEvent` |
| `card.action` | đồng bộ; áp card mới nếu `cardVersion` lớn hơn rồi phát realtime | `card.Service.RunAction` |
| `message.created` | **bất đồng bộ**, ghi `webhook_outbox`, worker retry backoff 2^n (tối đa 10 phút) | `internal/service/outbox` |
| `user.updated` | bất đồng bộ (dùng chung outbox) | — |

Chữ ký: `hex(HMAC_SHA256(CHAT_SERVICE_KEY, "<timestamp>.<rawBody>"))` tính trên **đúng bytes đã gửi**.
`eventId` duy nhất mỗi event; gửi lại giữ nguyên `eventId` để CMS chống trùng.

### 8. Bot API (CMS gọi Chat)

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

### 9. API cho FE

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

### 10. Module task nội bộ

`TASK_ENGINE_ENABLED=false` (mặc định): route `/api/v1/tasks`, `/teams`, `/revisions` **không** được
đăng ký, vì CMS là nguồn sự thật của đơn. Code + migration vẫn còn trong repo
(`internal/service/task`, `migrations/000003_task.*`) để dùng khi chạy độc lập, bật lại bằng env.

### 11. Đã kiểm chứng (CMS giả lập theo đúng tài liệu)

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

### 12. Còn thiếu / cần CMS thật để chốt

- Chưa test với CMS dev thật (đang chờ URL + `CHAT_SERVICE_KEY`); CMS giả lập dựng theo đúng payload
  trong tài liệu.
- `user.updated` đã có đường outbox nhưng chưa có chỗ phát (cần chốt khi nào Chat coi là hồ sơ đổi).
- SSE `GET /chat/stream`: đang dùng WebSocket (tài liệu cho phép chọn một trong hai).

---

## Bot @bot và mẫu (template)

### 1. Ý tưởng

Người dùng gọi bot trong chat bằng `@<slug>` rồi gửi một **mẫu** (form). Phase này bot **chỉ lưu
mẫu xuống DB** rồi trả lời xác nhận trong hội thoại. Việc "biến mẫu thành task/phiếu sửa" sẽ làm sau
mà **không phải sửa gì ở tầng chat** — chỉ cần một worker đọc bảng `bot_submissions`.

### 2. Hai cách gửi mẫu

**a) Gõ trong tin nhắn** (`message.send` hoặc `POST /conversations/:id/messages`):

```
@bot yeu_cau tieu_de="Banner Tet" to=design so_luong=3 deadline=2026-12-01
@bot phieu_sua {"ma_task":"T-100","nguyen_nhan":"sai mau","loai_sua":"mau","so_luong":2}
@bot                       -> bot trả về danh sách mẫu đang có (help)
```

Hỗ trợ cả `key=value` (có `"..."` cho giá trị chứa khoảng trắng) và JSON object.
Giá trị số/bool được tự nhận kiểu đúng khi lưu `jsonb`.

**b) Submit từ form** — FE lấy schema qua `GET /api/v1/bots/bot/templates`, render form, rồi gửi:

- WS: frame `template.send`
- REST: `POST /api/v1/bots/:slug/submissions`
- gRPC: `BotService.SubmitBotTemplate`

Cả hai cách đều đi vào **cùng một đường**: `message.Service.Send()` → `bot.Service.Dispatch()`,
nên luôn tạo đúng 1 tin nhắn `bot_command` của người dùng + 1 tin nhắn `bot_reply` của bot.

### 3. Dữ liệu trong DB

```
bots              (id, slug, name, description, enabled)
bot_templates     (id, bot_id, code, name, description, schema jsonb, enabled)
bot_submissions   (id, bot_id, bot_slug, template_id, template_code,
                   conversation_id, message_id, thread_id, user_id,
                   raw_text, payload jsonb, status, error, created_at)
```

`status`: `stored` (đã lưu — phase 1 dừng ở đây) → `processing` → `done` | `failed`.

`schema` của mẫu (FE dùng để render form, BE dùng để validate field bắt buộc):

```json
{"fields":[
  {"key":"tieu_de","label":"Tieu de","type":"text","required":true},
  {"key":"so_luong","label":"So luong","type":"number","required":true},
  {"key":"nguoi_phu_trach","label":"Nguoi phu trach","type":"user","required":false}
]}
```

`type` hiện chỉ để FE render: `text | textarea | number | date | user | select`.
Thiếu field `required` → bot trả lỗi `bot_template_missing_fields` kèm danh sách thiếu (trong
`error_details` của tin nhắn trả lời) và **không** lưu submission.

Seed sẵn trong `migrations/000002_seed_bot.up.sql`: bot `@bot` với 2 mẫu `yeu_cau`, `phieu_sua`.

### 4. Thêm mẫu mới

Chỉ là dữ liệu, không cần đổi code:

```bash
make migrate-create name=add_bot_template_goi_edit
```

```sql
-- migrations/0000xx_add_bot_template_goi_edit.up.sql
INSERT INTO bot_templates (bot_id, code, name, description, schema)
SELECT id, 'goi_edit', 'Goi Edit', 'Chuyen phan hinh da xong sang lam video',
       '{"fields":[{"key":"ma_task","label":"Ma task","type":"text","required":true},
                   {"key":"ghi_chu","label":"Ghi chu","type":"textarea","required":false}]}'::jsonb
FROM bots WHERE slug = 'bot'
ON CONFLICT (bot_id, code) DO NOTHING;
```

### 5. Thêm bot mới có logic riêng

Mặc định mọi bot dùng `TemplateHandler` (lưu mẫu). Muốn bot làm việc khác — ví dụ `@ai` gọi sang
AI service, `@drive` tạo thư mục — thì implement interface `bot.Handler`:

```go
// internal/service/bot/ai_handler.go
type AIHandler struct{ client AIClient }

func (h *AIHandler) Handle(ctx context.Context, req *bot.Request) (*bot.Result, error) {
    answer, err := h.client.Ask(ctx, req.RawText)
    if err != nil {
        return nil, apperr.Internal("ai_failed", "AI dang qua tai").Wrap(err)
    }
    return &bot.Result{ReplyText: answer, ReplyMeta: domain.JSONMap{"kind": "ai_answer"}}, nil
}
```

rồi đăng ký trong `internal/app/app.go`:

```go
botSvc := bot.New(store)
botSvc.Register("ai", bot.NewAIHandler(aiClient))   // "@ai ..."
```

và thêm 1 migration `INSERT INTO bots (slug, name, ...) VALUES ('ai', ...)`.

`Result.Silent = true` nếu bot không muốn trả lời trong chat.

### 6. Bước tiếp theo (khi làm task thật)

Worker đọc `bot_submissions WHERE status = 'stored'`, tạo task/phiếu sửa, rồi
`UPDATE status = 'done'`. Tầng chat không cần thay đổi; bot chỉ cần gửi thêm 1 tin nhắn trạng thái
vào `thread_id` của submission.

---

## Module task nội bộ

### 1. Mô hình dữ liệu

```
tasks                 1 yêu cầu/công việc (có mã T-000001, nằm trong 1 conversation + 1 thread)
  └─ task_assignments mỗi người được giao = 1 dòng ("card của ai hiện của người đó")
  └─ task_revisions   phiếu sửa (mã R-000001), hoàn thành riêng từng phiếu
  └─ task_events      lịch sử: ai làm gì, lúc nào, từ trạng thái nào sang trạng thái nào
teams / team_members  tổ (bộ phận) + thành viên, dùng để "chọn tổ" và gợi ý người phụ trách
```

Quy tắc quan trọng: **trạng thái tổng của task không được set tay**, nó được suy ra từ các card
(`domain.DeriveTaskStatus`). Nhờ vậy chia task cho nhiều người vẫn không bị lệch trạng thái.

| Trạng thái card (`task_assignments.status`) | Nghĩa |
|---|---|
| `pending` | đã giao, chưa nhận → hiện ở danh sách **Chờ nhận** |
| `in_progress` | đã nhận và đang làm |
| `paused` | tạm dừng |
| `on_hold` | treo (chờ đầu vào từ người khác) |
| `completed` | hoàn thành |
| `revoked` | bị thu hồi (do admin đổi người phụ trách / huỷ task) |

Luật suy trạng thái task: tất cả card `completed` → `completed`; còn ai `in_progress` → `in_progress`;
không ai đang làm mà có người `paused` → `paused`; có người `on_hold` → `on_hold`;
tất cả `pending` → `waiting_accept`; trộn (có người xong, người chưa nhận) → `in_progress`.

### 2. State machine của card

```
pending ──nhận──▶ in_progress ──tạm dừng──▶ paused ──tiếp tục──▶ in_progress
   │                  │  ▲                    │
   │                  │  └───────tiếp tục─────┘
   │                  ├──treo──▶ on_hold ──tiếp tục──▶ in_progress
   │                  └──hoàn thành──▶ completed ──(có phiếu sửa)──▶ in_progress
   └──admin đổi người──▶ revoked
```

Khai báo tại `internal/domain/task.go` (`AllowedAssignmentTransitions`). **Thêm trạng thái mới =
thêm 1 dòng ở map đó**, service không phải sửa. Chuyển sai trạng thái → lỗi `invalid_task_transition`
kèm `details.allowed` để FE biết nút nào được phép.

### 3. Luồng nghiệp vụ

#### Tạo yêu cầu (có gợi ý người phụ trách)

Hai cách, cùng đi vào `task.Service.Create`:

1. Gõ trong chat: `@bot yeu_cau tieu_de="Banner Tet" to=design so_luong=3 dinh_muc=A4 deadline=2026-12-01`
2. REST `POST /api/v1/tasks` / gRPC `TaskService.CreateTask` (FE render form từ schema của mẫu).

Khi tạo:
- Nếu **không chọn người phụ trách** và có `team_code`/`team_id` → hệ thống gợi ý: ưu tiên người
  **không OFF**, rồi người **ít việc đang mở nhất** (`CountOpenAssignments`), rồi theo tên cho ổn định.
  Task vẫn ghi `metadata.assignee_suggested = true` để FE hiển thị "gợi ý — có thể đổi người".
- Người được giao mà chưa ở trong nhóm chat sẽ **tự được thêm vào nhóm** (để thấy thread).
- Service đăng 1 **card** (tin nhắn hệ thống, `metadata.kind = "task_card"`) vào hội thoại. Nếu yêu cầu
  chưa có thread thì **card này chính là thread gốc** (`tasks.thread_id`), khớp spec "mỗi yêu cầu có
  thread riêng; phản hồi hiện cả trong nhóm và trong thread".

#### Nhận task

`POST /api/v1/tasks/:id/accept` với `{"quota":"A4-cao-cap","quantity":3}`.
Bắt buộc có **định mức + số lượng** trước khi nhận (đúng spec "Design chọn định mức và số lượng
trước khi nhận"); số lượng nhận không được vượt số lượng yêu cầu.

#### Xử lý task

`/pause`, `/resume`, `/hold` (bắt buộc ghi lý do), `/complete`. Mỗi thao tác chỉ tác động lên **card
của chính mình**; admin/tổ trưởng hoặc người tạo task có thể tác động card người khác bằng cách truyền
`assignment_id` (tin nhắn hệ thống sẽ ghi "Admin (thay Designer 1) ...").

Không hoàn thành được card khi **còn phiếu sửa đang mở** → lỗi `task_has_open_revision`.

#### Chia task / gọi hỗ trợ / gọi Edit

`POST /api/v1/tasks/:id/assignees` với `role`:
- `primary` — chia việc cho nhiều người, mỗi người 1 card riêng
- `support` — gọi hỗ trợ (khi người phụ trách OFF)
- `edit` — chuyển phần đã xong sang làm video, vẫn trong cùng thread

#### Điều chỉnh phân công

`POST /api/v1/tasks/:id/reassign` → thu hồi card cũ (`revoked`) + tạo card mới `pending` cho người mới.
`PATCH /api/v1/tasks/:id` → đổi tiêu đề/mô tả/tổ/số lượng/định mức/deadline/ưu tiên.
Chỉ **owner/admin của nhóm hoặc người tạo task** được làm (lỗi `require_task_manager`).

#### Phiếu sửa

- Tạo: `POST /api/v1/tasks/:id/revisions` hoặc `@bot phieu_sua ma_task=T-000001 nguyen_nhan="Sai mau" loai_sua=mau so_luong=2`.
  Bắt buộc có **nguyên nhân + loại sửa**; số lượng mặc định 1.
- Nếu card liên quan đã `completed` → tự **mở lại** thành `in_progress` để người phụ trách sửa.
- Hoàn thành **riêng từng phiếu**: `POST /api/v1/revisions/:revision_id/complete`. Người sửa (chủ card)
  hoặc admin được đóng phiếu; tin nhắn hệ thống ghi rõ còn bao nhiêu phiếu đang mở.

#### Chờ nhận

`GET /api/v1/tasks?waiting_accept=true` (thêm `mine=true` cho view của designer) — trả các task đã giao
nhưng chưa ai nhận card.

### 4. Mọi thay đổi đều để lại 3 dấu vết

1. **`task_events`** — lịch sử truy vết (`GET /api/v1/tasks/:id/events`).
2. **Tin nhắn hệ thống trong thread** — người trong nhóm đọc được diễn biến ngay trong chat.
3. **Realtime** — `task.created` và `task.updated` (payload `{task, event_type, ...}`) đẩy xuống mọi
   thành viên hội thoại qua WebSocket. FE chỉ cần vẽ lại card từ payload, không phải gọi lại API.

### 5. Bot: mẫu → nghiệp vụ

`internal/service/task/bot_handler.go` bọc `bot.TemplateHandler`:

```
@bot <mẫu> ... → TemplateHandler lưu bot_submissions (audit, status=stored)
               → yeu_cau   → task.Create     → submission = done, bot im lặng (card task là câu trả lời)
               → phieu_sua → task.CreateRevision
               → lỗi       → submission = failed + bot trả lời lý do trong chat
```

Thêm mẫu **có hành động**: thêm 1 `case` trong `BotHandler.Handle` + 1 migration `INSERT bot_templates`.
Mẫu chỉ cần lưu thì không phải sửa code (xem [Bot @bot và mẫu (template)](#bot-bot-và-mẫu-template)).

### 6. Giới hạn hiện tại (và chỗ làm tiếp)

- **Gọi hỗ trợ tự động khi người phụ trách OFF** chưa có job tự chạy: hiện admin gọi
  `/assignees` với `role=support`, còn `SuggestAssignees` đã loại người OFF. Khi làm tự động, đặt
  worker gọi đúng 2 hàm này.
- **Tính điểm / Drive** chưa làm: khi làm, đọc `task_assignments.quota + quantity` lúc `completed`
  (dữ liệu đã đủ, không cần đổi schema).
- `tasks.parent_task_id` đã có sẵn cho "gọi Edit tạo task con" nếu sau này muốn tách thành task riêng
  thay vì thêm card `role=edit`.

---

## Postman

Thư mục `internal/postman/` có **4 file**:

| File | Gửi cho / dùng để |
|---|---|
| `chat-service-fe.postman_collection.json` | **đội FE** — `FLOW · Luồng của FE` + `/api/client/v1/chat/*` + mẫu WebSocket |
| `chat-service-cms.postman_collection.json` | **đội CMS** — `FLOW · Luồng của CMS` + `/bot/v1/*` |
| `chat-service.postman_collection.json` | nội bộ — bản đầy đủ (81 request, 10 folder) |
| `chat-service.local.postman_environment.json` | environment dùng chung cho cả 3 |

Hai bản tách **dùng lại đúng các request của bản đầy đủ** (`gen_postman.py --check` đối chiếu, báo
lỗi nếu lệch), chỉ có folder `FLOW` là viết riêng theo góc nhìn của từng bên.

Cả hai sinh từ `scripts/gen_postman.py` (`make postman`). **Sửa API thì sửa script rồi chạy
lại, đừng sửa tay file JSON** — `make postman-lint` sẽ báo nếu file trên đĩa lệch với script.

> Trước đây còn một cây `internal/postman/collections/` định dạng v3 (mỗi request một file
> YAML) cho Postman CLI. Đã **xoá**: hai định dạng song song cho cùng một bộ API là trùng lặp.
> Mẫu WebSocket/gRPC — thứ duy nhất v2.1 không chứa được — giờ nằm trong folder
> **`10 · WebSocket & gRPC (mẫu frame)`** của chính file JSON, đọc ở tab Documentation.

### 1. Import và dùng (3 bước)

1. Chạy service: `make dev-up` (hoặc `make up-deps && make migrate-up && make run`).
2. Postman → **Import** → chọn 2 file:
   `internal/postman/chat-service.postman_collection.json` và `internal/postman/chat-service.local.postman_environment.json`.
3. Chọn environment **Chat Service · Local**, lấy token dev rồi dán vào biến `token`:

```bash
make token SUB=user-001 NAME="Nguyen Van A" ROLE=designer
```

**Mới dùng thì chạy folder `FLOW` trước** — 16 bước đánh số theo đúng thứ tự thật của hệ
thống (xem mục 3). Muốn tra cứu từng endpoint thì dùng các folder `00` → `10`.

Mỗi request tự lưu id cần thiết vào collection variable (`user_id`, `group_id`, `message_id`,
`thread_id`, `team_id`, `task_id`, `task_code`, `assignment_id`, `revision_id`) nên bước sau
dùng được ngay, không phải copy id bằng tay.

Muốn thử phân công 2 người (chia task, đổi người phụ trách, chat riêng): tạo token thứ 2
(`make token SUB=user-002 ROLE=order`), gọi `GET /api/v1/me` bằng token đó, rồi dán `user_id`
nhận được vào biến `user_id_2`.

### 2. Ba định danh dễ nhầm

Trước đây collection có `conversation_id`, `chat_id` và `team_id`. Hai cái đầu là **cùng một
nhóm chat**, chỉ khác dạng id — đã gộp thành **`group_id`**:

| Biến | Là gì | Dạng giá trị | Dùng ở |
|---|---|---|---|
| `group_id` | **nhóm chat** | UUID nội bộ **hoặc** `wg_<hex>` | cả 3 bề mặt: `/api/v1`, `/bot/v1`, `/api/client/v1` |
| `cms_chat_id` | chatId mà **CMS** muốn tạo | bắt buộc `wg_<hex>` | chỉ `PUT /bot/v1/groups/:id` |
| `team_id` | **tổ** của module task nội bộ | UUID | `/api/v1/teams/*` |

- `group_id` dùng được ở mọi nơi vì route hội thoại của `/api/v1` nhận **cả hai dạng id**
  (`convIDParam` trong `internal/transport/httpserver/handler/conversation.go`): là UUID thì
  dùng luôn, không phải UUID thì tra `conversations.external_id`.
- `cms_chat_id` phải tách riêng vì tham số URL của `PUT /bot/v1/groups/:id` **chính là** chatId
  cần tạo — truyền UUID vào sẽ đẻ ra một nhóm có `external_id` là chuỗi UUID đó.
- `team_id` là **thực thể khác hẳn** (bảng `teams`), không gộp được. Cũng đừng nhầm với `teamId`
  trong body `POST /forms/create_task/submit` — đó là **tổ của CMS**, dạng số (vd `"9"` — lấy từ `groups[].cms.teams[].id`), Chat
  chuyển nguyên sang CMS chứ không diễn giải.

Hệ quả: `GET /api/v1/conversations/<chuỗi-bất-kỳ>` không còn trả `400 invalid_uuid` mà trả
`404 conversation_not_found` — vì chuỗi đó giờ được hiểu là một `external_id` có thể có.

### 3. Folder FLOW — chạy từ trên xuống để hiểu luồng

16 bước đánh số; hai bước **cùng cấp** thì `1.1`, `1.2`…

| Giai đoạn | Bước |
|---|---|
| 1 · chuẩn bị | `1.1` healthz · `1.2` `/me` (lưu `user_id`) |
| 2 · nhóm & thành viên | `2.1` bootstrap (lưu `group_id` + `team_id_cms`) · `2.2` xem thành viên · `2.3` CMS đẩy nhóm sang |
| 3 · tin nhắn | `3.1` gửi · `3.2` trả lời thread · `3.3` đọc lịch sử · `3.4` đánh dấu đã đọc |
| 4 · **vòng đời một đơn** | `4.1` FE gửi form → CMS · `4.2` kiểm outbox · `4.3` **CMS đẩy card vào nhóm** · `4.4` FE bấm nút · `4.5` CMS cập nhật đơn (gộp 3 việc) |
| 5 · thông báo | `5.1` xem chưa đọc · `5.2` đánh dấu đã đọc |
| 6 · dọn dẹp | `6.1` xoá tin nhắn |

Giai đoạn 4 là chỗ hay hiểu sai: `4.1` **không tạo tin nhắn nào trong Chat** — card chỉ xuất hiện
ở `4.3` khi CMS gọi ngược vào `POST /bot/v1/messages`. Ở local CMS không gọi tới `localhost` được
nên `4.3` **đóng vai CMS** bằng `service_key`.

**Dừng ở một bước để xem kỹ**: đặt biến `stop_at` = số bước rồi Run collection.

⚠️ Khi `CMS_ENABLED=true`, bước `4.1` gọi sang CMS thật và **tạo đơn thật** ở đó.

```bash
make postman-flow TOKEN=$(make -s token SUB=demo-an) KEY=$CHAT_SERVICE_KEY STOP=2.3
# chạy 1.1 → 2.3 rồi dừng, in: "DUNG tai buoc 2.3 (stop_at). Doi stop_at de di tiep."
```

Để trống `STOP` thì chạy hết 16 bước. Bước `2.2` cố ý **không** đổi `group_id`: nhóm CMS đẩy
sang chỉ có thành viên do CMS khai, token của bạn không ở trong đó nên các bước `3.x` sẽ 403.

### 4. Nội dung collection (81 request / 10 folder)

| Folder | Nội dung |
|---|---|
| `FLOW` | 17 bước đánh số, chạy tuần tự (gồm vòng đời một đơn) — xem mục 3 |
| `00 · Bắt đầu` | healthz, readyz, `/me` (lưu `user_id`) |
| `01 · Hội thoại` | tạo nhóm / chat riêng, danh sách, chi tiết, thêm thành viên |
| `02 · Tin nhắn` | gửi text / ảnh / trong thread, lịch sử, tìm kiếm, đã đọc, ai đã xem, xoá |
| `04 · Bot @bot & gửi mẫu` | schema mẫu, `@bot` help, `@bot yeu_cau`, submit mẫu từ form, mẫu đã lưu |
| `07 · Lỗi & quyền` | 401 thiếu token, 404 id nhóm không có, 403 không phải thành viên, 404 route sai, 400 tin nhắn rỗng |
| `99 · Dọn dẹp` | rời nhóm / xoá thành viên (chạy sau cùng) |
| `08 · CMS → Chat (Bot API)` | CMS gọi sang Chat: resolve user, gửi/sửa card, thread, thông báo, đẩy nhóm |
| `09 · API cho FE (hợp đồng CMS)` | `/api/client/v1/chat/*`: bootstrap, tin nhắn, bấm nút card, gửi form, thông báo |

Mỗi folder cần cấu hình khác nhau:

| Folder | Cần gì |
|---|---|
| `FLOW`, `00`–`04`, `07`, `99` | chạy với cấu hình mặc định |
| `08`, `09` (CMS) | `CMS_ENABLED=true` + biến `service_key`, `cms_token`, `group_id` — xem [Tích hợp CMS (Designer Bot)](#tích-hợp-cms-designer-bot) |

```bash
make postman-flow TOKEN=$(make -s token) KEY=$CHAT_SERVICE_KEY [STOP=2.3]   # folder FLOW
make postman-run TOKEN=$(make -s token)                 # tất cả trừ folder CMS
make postman-run-cms TOKEN=<access token CMS> KEY=<CHAT_SERVICE_KEY> CHAT_ID=wg_...
```

#### Đã lược bỏ: module task nội bộ

27 request của `03 · Tổ`, `05 · Task`, `06 · Phiếu sửa` đã **xoá khỏi collection và khỏi
`scripts/gen_postman.py`**. Lý do: các route `/api/v1/tasks/*`, `/api/v1/teams/*`,
`/api/v1/revisions/*` chỉ được đăng ký khi `TASK_ENGINE_ENABLED=true` (`router.go`
`registerTaskRoutes`), còn khi tích hợp CMS thì đơn/task do CMS sở hữu — `.env` để `false`
nên chúng luôn trả `404 route_not_found`.

Code Go của module task **vẫn còn** (xem [Module task nội bộ](#module-task-nội-bộ)); chỉ phần Postman bị bỏ.
Nếu bật lại `TASK_ENGINE_ENABLED=true` thì phải viết lại các request đó.

Các biến chỉ dùng cho module này cũng đã bỏ: `team_id`, `task_id`, `task_code`,
`assignment_id`, `revision_id`.

Mỗi request có mô tả (ràng buộc, mã lỗi có thể gặp) và test script kiểm tra status + trường quan trọng.

### 5. Chạy tự động (CI)

```bash
make postman-run TOKEN=$(make -s token) USER2=<uuid-nguoi-thu-2>
```

Kết quả lần chạy mới nhất trên stack dev: FLOW **17/17**, folder thường **50/50**,
folder CMS **37/37** — 0 fail.

`make postman-lint` kiểm file JSON: request trùng tên, thiếu method/URL, body không phải JSON
hợp lệ, và **file trên đĩa lệch với script sinh ra nó** (sửa tay trong app rồi quên cập nhật
script là lỗi hay gặp nhất).

### 6. WebSocket & gRPC trong app Postman

Định dạng v2.1 không chứa được 2 protocol này
([tài liệu Postman](https://learning.postman.com/docs/getting-started/importing-and-exporting/exporting-data)),
nên mẫu nằm trong **mô tả** của folder `10 · WebSocket & gRPC (mẫu frame)`: mở folder trong
Postman → tab **Documentation** → copy khối JSON cần dùng.

Trong đó có sẵn:

- **2 URL WebSocket** và khác nhau ở đâu (`/api/client/v1/chat/ws` cho FE, `/ws` cho công cụ nội bộ)
- **8 frame gửi lên** có body đầy đủ: `ping`, `message.send` (text / thread / kèm tệp),
  `message.read`, `typing`, `message.delete`, `template.send`
- **frame nhận về**: `ready`, `ack`, `message`, và bảng 7 event còn lại
- **bảng 13 trường hợp hay gặp**: thiếu token, token hết hạn, không phải thành viên,
  `clientMsgId` trùng, JSON sai, frame lạ, ping/pong timeout, mất kết nối…
- **5 message mẫu cho gRPC** + cách gắn metadata `authorization`

Giao thức đầy đủ: [Giao thức WebSocket](frontend.md#giao-thức-websocket).

### 7. Khi API đổi

1. Sửa `scripts/gen_postman.py` (thêm/sửa request, mô tả, assertion).
2. `make postman` → sinh lại 2 file JSON.
3. `make postman-lint` và `make postman-run TOKEN=...` để chắc chắn còn chạy đúng.



---

---

## Hướng dẫn test

Ba cách, từ nhanh đến đầy đủ. Mọi cách đều dùng chung dữ liệu do `make seed` dựng sẵn.

### 0. Khởi động + dựng dữ liệu (1 lần)

```bash
make dev-up      # Postgres + Redis + service (http://localhost:8080)
make seed        # 3 người dùng, 1 nhóm, tin nhắn + reply + thread + 1 yêu cầu @bot
```

`make seed` in ra: `conversation_id`, id tin gốc có thread, **token 7 ngày của 3 người** và **URL
WebSocket dán thẳng được**. Chạy lại bao nhiêu lần cũng được (tạo thêm nhóm mới, không phá dữ liệu cũ).

Trỏ sang máy khác: `make seed BASE=http://<vps>:8080`.

### 1. Nhanh nhất — chat trên terminal

Mở 2 cửa sổ, mỗi cửa sổ một người (lấy `CONV` từ output của `make seed`):

```bash
make ws SUB=demo-an   NAME=An   CONV=<conversation_id>
make ws SUB=demo-binh NAME=Binh CONV=<conversation_id>
```

Gõ chữ rồi Enter → cửa sổ kia hiện ngay. Lệnh trong client: `/ping`, `/typing`,
`/read <message_id>`, `/json {...}` (gửi frame bất kỳ), `/quit`.

### 2. Postman — bấm thử từng API

Cách import, cấu hình biến và chạy bằng CLI: xem [Postman](#postman) ở trên. Tóm lại:

```bash
make postman-flow TOKEN=$(make -s token SUB=demo-an) [STOP=2.3]   # 16 bước theo thứ tự thật
make postman-run  TOKEN=$(make -s token SUB=demo-an)              # tất cả trừ folder CMS
```

Kết quả mong đợi: FLOW **17/17**, folder thường **50/50**, folder CMS **37/37** — 0 fail.

**Test WebSocket**: mở folder `10 · WebSocket & gRPC (mẫu frame)` → tab Documentation → copy
URL + frame. Mở nhiều tab cho nhiều người, **Connect hết rồi mới gửi** (WebSocket không phát
lại tin cũ).

> Đừng đặt `clientMsgId` cố định như `"m1"`: gửi lại giá trị cũ bị coi là retry → server trả
> `duplicated: true` và **không phát cho ai**. Bỏ trống là an toàn nhất khi test tay.

Tab nào nghi đã chết thì gửi `{"id":"x","type":"ping"}` — có `pong` là còn sống. Hoặc gọi
`GET /api/v1/ws/online` để xem ai đang kết nối.

### 3. curl — kiểm tra từng thứ

```bash
TOKEN=<token từ make seed>
CONV=<conversation_id>

curl -s localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/conversations?limit=10" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/conversations/$CONV/messages?limit=20" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/messages/<message_id>/replies?mode=thread" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/messages/search?q=banner&conversation_id=$CONV" -H "Authorization: Bearer $TOKEN" | jq
curl -s -X POST "localhost:8080/api/v1/conversations/$CONV/read" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"message_id":"<message_id>"}' | jq
```

API cho FE (envelope `{statusCode, code, message, element}`): đổi tiền tố thành
`/api/client/v1/chat/...`, ví dụ `GET /api/client/v1/chat/bootstrap`.

### 4. Test code (không cần service chạy)

```bash
make test            # unit test toàn bộ
make lint            # golangci-lint (cần cài: make setup)
make dev-test        # chạy test bên trong container dev
```

### 5. Khi có lỗi — nhìn đâu

| Hiện tượng | Lệnh / chỗ xem |
|---|---|
| Request lỗi 4xx/5xx | `make dev-logs` — log JSON có `request_id`, `path`, `status` |
| Gửi tin mà người khác không nhận | `make dev-logs \| grep "deliver event"` → `delivered_connections` và `not_connected_here` |
| Không chắc tab WebSocket còn sống | `GET /api/v1/ws/online` hoặc gửi frame `ping` |
| Tin nhắn "biến mất" | tìm `bo qua tin nhan trung client_msg_id` trong log (trùng `client_msg_id`) |
| Nghi sai dữ liệu | `make dev-psql` rồi `select id, content, reply_to_id, thread_id from messages order by created_at desc limit 10;` |

### 6. Dọn dẹp

```bash
make dev-down        # dừng, giữ dữ liệu
make dev-down-v      # dừng + xoá sạch dữ liệu (chạy make seed lại để có bộ mới)
```
