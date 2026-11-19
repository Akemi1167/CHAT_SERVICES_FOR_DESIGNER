# 05 — API

Tất cả REST endpoint (trừ health) yêu cầu `Authorization: Bearer <JWT>`.
Response có dạng `{"data": ..., "meta": {...}}` hoặc `{"error":{"code","message","details"}}`.
`meta` của endpoint danh sách là `{"next_cursor":"...","has_more":true}`.

## REST

> `:id` của mọi route hội thoại nhận **cả hai dạng id của cùng một nhóm**: UUID nội bộ
> hoặc `chatId` của CMS (`wg_<hex>` = `conversations.external_id`). Nhờ vậy `/api/v1`,
> `/bot/v1` và `/api/client/v1` dùng chung một giá trị id.

| Method | Path | Mô tả |
|---|---|---|
| GET | `/healthz` | liveness |
| GET | `/readyz` | readiness (ping DB) |
| GET | `/ws?token=` | WebSocket, hình dạng nội bộ (snake_case) |
| GET | `/api/client/v1/chat/ws?token=` | WebSocket cho FE — **cùng access token CMS với REST**, field camelCase trùng REST (xem [02-websocket.md](02-websocket.md)) |
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

### Task (xem [07-task.md](07-task.md))

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

### Tổ / bộ phận

| Method | Path | Mô tả |
|---|---|---|
| GET | `/api/v1/teams` | danh sách tổ (seed: `design`, `edit`, `order`) |
| POST | `/api/v1/teams` | tạo tổ — `{"code","name","kind"}` |
| GET | `/api/v1/teams/:id/members` | thành viên tổ |
| POST | `/api/v1/teams/:id/members` | thêm thành viên — `{"user_ids":[...],"role":"leader|assistant|member"}` |
| DELETE | `/api/v1/teams/:id/members/:user_id` | bỏ thành viên khỏi tổ |
| GET | `/api/v1/teams/:id/suggest` | **gợi ý người phụ trách** (không OFF + ít việc nhất trước) |

WebSocket: `GET /ws?token=<JWT>` — xem [02-websocket.md](02-websocket.md).

## gRPC (`api/proto/chat/v1`)

Auth: metadata `authorization: Bearer <JWT>`. Lỗi trả về theo `codes` chuẩn
(`InvalidArgument`, `Unauthenticated`, `PermissionDenied`, `NotFound`, `AlreadyExists`, `Internal`).

`ChatService`: `CreateConversation`, `GetConversation`, `ListConversations`, `AddMembers`,
`RemoveMember`, `SendMessage`, `ListMessages`, `SearchMessages`, `MarkRead`.

Bot API (CMS gọi): thêm `POST /bot/v1/messages/:id/pin`, `PUT /bot/v1/presence`,
`GET /bot/v1/attachments/:id` — xem [10-cms-integration.md](10-cms-integration.md) §8.

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
