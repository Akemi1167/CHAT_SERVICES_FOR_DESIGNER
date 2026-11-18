# 03 — Bot `@bot` và "gửi mẫu"

## 1. Ý tưởng

Người dùng gọi bot trong chat bằng `@<slug>` rồi gửi một **mẫu** (form). Phase này bot **chỉ lưu
mẫu xuống DB** rồi trả lời xác nhận trong hội thoại. Việc "biến mẫu thành task/phiếu sửa" sẽ làm sau
mà **không phải sửa gì ở tầng chat** — chỉ cần một worker đọc bảng `bot_submissions`.

## 2. Hai cách gửi mẫu

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

## 3. Dữ liệu trong DB

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

## 4. Thêm mẫu mới

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

## 5. Thêm bot mới có logic riêng

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

## 6. Bước tiếp theo (khi làm task thật)

Worker đọc `bot_submissions WHERE status = 'stored'`, tạo task/phiếu sửa, rồi
`UPDATE status = 'done'`. Tầng chat không cần thay đổi; bot chỉ cần gửi thêm 1 tin nhắn trạng thái
vào `thread_id` của submission.
