# 07 — Task: tạo yêu cầu, nhận task, phiếu sửa

## 1. Mô hình dữ liệu

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

## 2. State machine của card

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

## 3. Luồng nghiệp vụ

### Tạo yêu cầu (có gợi ý người phụ trách)

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

### Nhận task

`POST /api/v1/tasks/:id/accept` với `{"quota":"A4-cao-cap","quantity":3}`.
Bắt buộc có **định mức + số lượng** trước khi nhận (đúng spec "Design chọn định mức và số lượng
trước khi nhận"); số lượng nhận không được vượt số lượng yêu cầu.

### Xử lý task

`/pause`, `/resume`, `/hold` (bắt buộc ghi lý do), `/complete`. Mỗi thao tác chỉ tác động lên **card
của chính mình**; admin/tổ trưởng hoặc người tạo task có thể tác động card người khác bằng cách truyền
`assignment_id` (tin nhắn hệ thống sẽ ghi "Admin (thay Designer 1) ...").

Không hoàn thành được card khi **còn phiếu sửa đang mở** → lỗi `task_has_open_revision`.

### Chia task / gọi hỗ trợ / gọi Edit

`POST /api/v1/tasks/:id/assignees` với `role`:
- `primary` — chia việc cho nhiều người, mỗi người 1 card riêng
- `support` — gọi hỗ trợ (khi người phụ trách OFF)
- `edit` — chuyển phần đã xong sang làm video, vẫn trong cùng thread

### Điều chỉnh phân công

`POST /api/v1/tasks/:id/reassign` → thu hồi card cũ (`revoked`) + tạo card mới `pending` cho người mới.
`PATCH /api/v1/tasks/:id` → đổi tiêu đề/mô tả/tổ/số lượng/định mức/deadline/ưu tiên.
Chỉ **owner/admin của nhóm hoặc người tạo task** được làm (lỗi `require_task_manager`).

### Phiếu sửa

- Tạo: `POST /api/v1/tasks/:id/revisions` hoặc `@bot phieu_sua ma_task=T-000001 nguyen_nhan="Sai mau" loai_sua=mau so_luong=2`.
  Bắt buộc có **nguyên nhân + loại sửa**; số lượng mặc định 1.
- Nếu card liên quan đã `completed` → tự **mở lại** thành `in_progress` để người phụ trách sửa.
- Hoàn thành **riêng từng phiếu**: `POST /api/v1/revisions/:revision_id/complete`. Người sửa (chủ card)
  hoặc admin được đóng phiếu; tin nhắn hệ thống ghi rõ còn bao nhiêu phiếu đang mở.

### Chờ nhận

`GET /api/v1/tasks?waiting_accept=true` (thêm `mine=true` cho view của designer) — trả các task đã giao
nhưng chưa ai nhận card.

## 4. Mọi thay đổi đều để lại 3 dấu vết

1. **`task_events`** — lịch sử truy vết (`GET /api/v1/tasks/:id/events`).
2. **Tin nhắn hệ thống trong thread** — người trong nhóm đọc được diễn biến ngay trong chat.
3. **Realtime** — `task.created` và `task.updated` (payload `{task, event_type, ...}`) đẩy xuống mọi
   thành viên hội thoại qua WebSocket. FE chỉ cần vẽ lại card từ payload, không phải gọi lại API.

## 5. Bot: mẫu → nghiệp vụ

`internal/service/task/bot_handler.go` bọc `bot.TemplateHandler`:

```
@bot <mẫu> ... → TemplateHandler lưu bot_submissions (audit, status=stored)
               → yeu_cau   → task.Create     → submission = done, bot im lặng (card task là câu trả lời)
               → phieu_sua → task.CreateRevision
               → lỗi       → submission = failed + bot trả lời lý do trong chat
```

Thêm mẫu **có hành động**: thêm 1 `case` trong `BotHandler.Handle` + 1 migration `INSERT bot_templates`.
Mẫu chỉ cần lưu thì không phải sửa code (xem [03-bot-template.md](03-bot-template.md)).

## 6. Giới hạn hiện tại (và chỗ làm tiếp)

- **Gọi hỗ trợ tự động khi người phụ trách OFF** chưa có job tự chạy: hiện admin gọi
  `/assignees` với `role=support`, còn `SuggestAssignees` đã loại người OFF. Khi làm tự động, đặt
  worker gọi đúng 2 hàm này.
- **Tính điểm / Drive** chưa làm: khi làm, đọc `task_assignments.quota + quantity` lúc `completed`
  (dữ liệu đã đủ, không cần đổi schema).
- `tasks.parent_task_id` đã có sẵn cho "gọi Edit tạo task con" nếu sau này muốn tách thành task riêng
  thay vì thêm card `role=edit`.
