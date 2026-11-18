# 04 — Thêm / sửa / xoá field & API (checklist)

Nguyên tắc: **`migrations/` là nguồn sự thật của schema**, `internal/domain` là nguồn sự thật của
entity, `api/proto` là nguồn sự thật của API gRPC. Không bao giờ sửa file migration đã chạy;
luôn tạo file mới.

## A. Thêm 1 field (ví dụ: `messages.pinned`)

1. **Migration**
   ```bash
   make migrate-create name=add_pinned_to_messages
   ```
   ```sql
   -- up
   ALTER TABLE messages ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT false;
   CREATE INDEX IF NOT EXISTS ix_msg_pinned ON messages (conversation_id) WHERE pinned;
   -- down
   DROP INDEX IF EXISTS ix_msg_pinned;
   ALTER TABLE messages DROP COLUMN IF EXISTS pinned;
   ```
   ```bash
   make migrate-up
   ```

2. **Domain** — `internal/domain/message.go`
   ```go
   Pinned bool `gorm:"default:false;index" json:"pinned"`
   ```

3. **Repository** — thêm method nếu cần query mới (ví dụ `ListPinned`), khai báo ở
   `internal/repository/repository.go` rồi implement trong `internal/repository/postgres/message_repo.go`.

4. **Service** — thêm logic + kiểm tra quyền (`internal/service/message/service.go`).

5. **Transport**
   - REST: thêm field vào request struct trong `internal/transport/httpserver/handler/*.go`
     (response trả `domain.Message` nên field mới tự xuất hiện qua tag `json`).
   - gRPC: thêm field vào `api/proto/chat/v1/message.proto` (**số field mới, không tái dùng số cũ**),
     `make proto`, rồi map trong `internal/transport/grpcserver/mapper.go`.
   - WS: nếu là frame mới thì thêm hằng số + payload struct trong `ws/protocol.go` và 1 `case`
     trong `ws/handler.go`.

6. `make fmt test` và xong.

**Đường tắt:** nếu field chỉ để hiển thị / chưa chắc chắn, bỏ vào `metadata` (jsonb) của message
hoặc conversation — không cần migration, không cần sửa proto. Khi field đã ổn định và cần index
thì mới tách thành cột riêng.

## B. Sửa field

- Đổi **tên**: tạo migration `ALTER TABLE ... RENAME COLUMN`, sửa tag GORM trong domain. Với gRPC,
  **giữ nguyên số field**, chỉ đổi tên (số field mới là hợp đồng nhị phân, tên thì không).
- Đổi **kiểu**: làm 3 bước để không downtime — (1) thêm cột mới + backfill, (2) deploy code ghi/đọc
  cả hai, (3) migration xoá cột cũ.
- Đổi **ràng buộc** (`NOT NULL`, `CHECK`): nhớ backfill dữ liệu cũ trước khi thêm ràng buộc.

## C. Xoá field

1. Deploy code không còn dùng field (xoá khỏi service/transport, giữ cột trong DB).
2. Proto: **đánh dấu `reserved`**, đừng xoá hẳn số field:
   ```proto
   message Message { reserved 9; reserved "metadata_json"; }
   ```
3. Migration tiếp theo mới `DROP COLUMN`.

## D. Thêm 1 API mới

| Loại | Việc cần làm |
|---|---|
| REST | thêm handler trong `handler/`, đăng ký route trong `httpserver/router.go` |
| gRPC | thêm `rpc` vào `api/proto/chat/v1/service.proto` → `make proto` → thêm method vào `grpcserver/*_handler.go` |
| WS | thêm hằng `In...` + payload struct trong `ws/protocol.go` → thêm `case` trong `ws/handler.go` |

Nghiệp vụ **luôn** đặt ở `internal/service/...` để cả 3 transport dùng chung; handler chỉ parse và map.

## E. Thêm bảng mới

1. Migration `CREATE TABLE ...`
2. Struct trong `internal/domain/` + thêm vào `database.AllModels()` (để AutoMigrate ở local khớp).
3. Interface repository + implement GORM.
4. Service + transport theo nhu cầu.

## E2. Thêm 1 trạng thái task mới (ví dụ `waiting_qc`)

1. `internal/domain/task.go`: thêm hằng `AssignWaitingQC` + 1 dòng trong `AllowedAssignmentTransitions`
   (từ trạng thái nào sang được) và nhánh trong `DeriveTaskStatus` nếu ảnh hưởng trạng thái tổng.
2. Migration: nới `CHECK` constraint của `task_assignments.status` / `tasks.status`.
3. `internal/service/task/render.go`: thêm nhãn tiếng Việt.
4. `internal/service/task/lifecycle.go`: thêm 1 hàm ngắn gọi `applyTransition` (5-10 dòng).
5. Transport: thêm route REST + 1 RPC dùng lại `TaskActionRequest`.
6. `internal/domain/task_test.go`: thêm case vào bảng test.

## F. Hay quên

- `POSTGRES_AUTO_MIGRATE=true` chỉ dùng ở local; prod luôn `make migrate-up`.
- Mọi lỗi trả về FE phải là `apperr` (có `code` ổn định để FE bắt), không trả lỗi GORM thô.
- Query timeline phải dùng cursor (`paging.Page`), đừng dùng `OFFSET`.
- Sau khi sửa `.proto`: `make proto` rồi `make test` — mapper không khớp sẽ fail compile ngay.
