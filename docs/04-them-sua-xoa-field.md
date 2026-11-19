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

## G. Migrate có mất dữ liệu không?

**Chiều lên (`make mu` / `make migrate-up`): không.** Toàn bộ `*.up.sql` trong repo chỉ có
`CREATE TABLE` / `ADD COLUMN` / `CREATE INDEX` / `INSERT` (seed bot). Ngoại lệ duy nhất là
`000005_fix_cms_unique_empty.up.sql`: nó `DROP INDEX` rồi tạo lại ngay (index không chứa dữ liệu),
và `UPDATE users SET open_id = NULL WHERE open_id = ''` — sửa giá trị rác, không xoá hàng.

Kiểm lại bất cứ lúc nào:

```bash
grep -nE "^\s*(DROP|TRUNCATE|DELETE)" migrations/*.up.sql
```

**Chiều xuống (`make mdown` / `make migrate-down`): CÓ, mất thật.**
`DROP COLUMN` của Postgres không undo được. Rollback rồi migrate lên lại thì cột quay về
nhưng **rỗng**.

| Lùi từ → đến | File chạy | Mất gì |
|---|---|---|
| 6 → 5 | `000006.down` | cột `messages.pinned`, `pinned_at`, `pinned_by`, `users.presence_reason` |
| 5 → 4 | `000005.down` | chỉ đổi index — **không mất dữ liệu** |
| 4 → 3 | `000004.down` | **bảng** `notifications`, `webhook_outbox`, `idempotency_keys` + 13 cột CMS (`card`, `cms_profile`, `external_id`…) |
| 3 → 2 | `000003.down` | **bảng** `tasks`, `task_events`, `task_revisions`, `teams`, `team_members` |
| 2 → 1 | `000002.down` | bản ghi seed bot `@bot` |
| 1 → 0 | `000001.down` | **toàn bộ schema**: `messages`, `conversations`, `users`… |

Quy tắc:

- **Local**: `mdown` thoải mái, dữ liệu chỉ là seed (`make seed` dựng lại được).
- **Prod**: coi `mdown` như lệnh cấm. Sửa sai thì viết migration **mới** đi tiếp lên, đừng lùi.
  Bắt buộc lùi thì backup trước: `make prod-backup OUT=/backup/truoc-rollback.sql`.

**Hai lệnh mất nhiều hơn cả `mdown`**, dễ nhầm là của migrate:

```bash
make down-v        # docker compose down -v -> xoá luôn volume, mất sạch database
make dev-down-v    # tương tự cho stack dev
```

`make down` (không `-v`) thì an toàn — chỉ dừng container, volume còn nguyên.
