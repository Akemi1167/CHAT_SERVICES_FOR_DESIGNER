# 06 — Phần còn lại của spec map vào kiến trúc

Spec Designer Bot còn nhiều tính năng ngoài phạm vi phase này. Dưới đây là chỗ đặt từng phần khi
làm tiếp, để không phải đổi kiến trúc.

| Tính năng trong spec | Đã có gì | Làm tiếp ở đâu |
|---|---|---|
| Chat nhóm theo bộ phận, avatar user | **xong** | — |
| Gửi chữ/ảnh/video/GIF xen kẽ | **xong** (`attachments` + `type`) | upload file là service riêng; chat chỉ lưu URL |
| Thread theo nhiệm vụ | **xong** (`messages.thread_id`) | — |
| Trạng thái đã xem / danh sách người đã đọc | **xong** (`last_read_at`, `message_receipts`) | — |
| Trạng thái OFF cạnh tên | **một phần** (`users.presence`) | job đồng bộ lịch OFF → `presence='off_duty'` |
| Tìm tin nhắn / tệp trong nhóm | **xong** cho tin nhắn | thêm endpoint `GET /conversations/:id/files` query bảng `attachments` |
| Tạo yêu cầu trong chat, gợi ý người phụ trách | **xong** (`internal/service/task`, gợi ý theo tổ + tải việc + OFF) | — |
| Nhận task / tạm dừng / treo / hoàn thành | **xong** (state machine ở `domain.AllowedAssignmentTransitions`) | — |
| Phiếu sửa | **xong** (`task_revisions`, hoàn thành riêng từng phiếu) | — |
| Gọi hỗ trợ khi OFF, phân công theo pool | **một phần**: `role=support` + gợi ý đã loại người OFF | worker tự gọi hỗ trợ khi phát hiện OFF |
| Gọi Edit, chia task nhiều người | **xong** (`role=edit`, mỗi người 1 card) | tách task con qua `tasks.parent_task_id` nếu cần |
| Chờ nhận, điều chỉnh phân công | **xong** (`?waiting_accept=true`, `/reassign`, `PATCH /tasks/:id`) | — |
| Role Admin/Order/Designer hiển thị khác nhau | `users.role` + `conversation_members.role` + `requireManager` cho task | tinh chỉnh thêm theo từng màn hình FE |
| Tính điểm theo định mức/số lượng | dữ liệu đã có ở `task_assignments.quota/quantity` khi `completed` | service tính điểm + đồng bộ Drive |
| Đơn hàng, Drive, tính điểm, Drive Assets | — | service ngoài, gọi vào chat qua **gRPC** (`ChatService.SendMessage`) để post tin nhắn hệ thống |
| Tick dùng AI | — | bot riêng: `botSvc.Register("ai", ...)` — xem [03-bot-template.md](03-bot-template.md) §5 |

Hai mở rộng kỹ thuật nên tính trước:

- **Outbox cho event**: nếu sau này không được mất event realtime, thêm bảng `outbox_events` ghi
  trong cùng transaction với message, rồi worker publish (thay cho `hub.Publish` trực tiếp).
- **Partition bảng `messages`** theo tháng khi dữ liệu lớn; index hiện tại
  (`conversation_id, created_at DESC, id DESC`) đã sẵn cho kiểu truy vấn này.
