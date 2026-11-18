# 15 — Nối Chat vào CMS (từng bước)

Làm theo đúng thứ tự: **xin thông tin → đưa thông tin → điền env → `make cms-check` → bật**.
Nền tảng kỹ thuật ở [10-cms-integration.md](10-cms-integration.md); file này chỉ là quy trình cắm dây.

## Bước 1 — Xin đội CMS 4 giá trị

| Cần | Là gì | Ví dụ | Hỏi thế nào |
|---|---|---|---|
| `CHAT_SERVICE_KEY` | **khoá dùng chung** cho cả 3 hướng (ký webhook, Chat gọi Workspace API, CMS gọi Bot API) | `9f2c…` | "Cho mình giá trị `CHAT_SERVICE_KEY` trong `.env` của CMS" |
| Base URL Workspace API | nơi Chat đọc nhóm/user | `https://cms.cty.com/api/client/v1/workspace` | "Base của Workspace API trên môi trường dev/prod?" |
| URL webhook của bot | nơi Chat gửi event | `https://cms.cty.com/webhook/web/designer` | "`webhookPath` của bot Web là gì?" |
| `iss` / `aud` của token | để Chat kiểm token người dùng | `designer-cms` / `abc-workspace` | "Token các bạn phát có `iss`/`aud` là gì?" (mặc định đúng như ví dụ) |

Hỏi thêm cho chắc: **CMS gọi Bot API của Chat bằng domain nào** — cần mở firewall cho IP đó.

## Bước 2 — Đưa đội CMS 2 giá trị

| Đưa | Giá trị | Họ dùng để |
|---|---|---|
| `CHAT_API_BASE` | `https://chat.cty.com/bot/v1` | gọi Bot API của Chat (gửi card, thông báo, đẩy nhóm) |
| `CHAT_SERVICE_KEY` | **cùng một khoá** ở bước 1 | gắn header `X-Chat-Service-Key` |

Nói rõ với họ: Chat đã làm đủ 10 endpoint Bot API (`users/resolve`, `messages`, `PATCH messages/:id`,
`messages/:id/thread`, `messages/:id/pin`, `notifications`, `presence`, `groups/:id`,
`groups/:id/members`, `attachments/:id`), có xử lý `Idempotency-Key`, và lỗi trả dạng
`{code, message, retryable}`.

## Bước 3 — Điền `.env`

```bash
CMS_ENABLED=true
CHAT_SERVICE_KEY=<khoá ở bước 1>
CMS_BASE_URL=https://cms.cty.com
CMS_WORKSPACE_BASE_URL=https://cms.cty.com/api/client/v1/workspace
CMS_WEBHOOK_URL=https://cms.cty.com/webhook/web/designer
CMS_JWT_ISSUER=designer-cms
CMS_JWT_AUDIENCE=abc-workspace
CMS_JWKS_URL=                      # để trống = <workspace>/.well-known/jwks.json
CMS_SYNC_INTERVAL=60s

TASK_ENGINE_ENABLED=false          # CMS sở hữu đơn/task
AUTH_ALLOW_DEV_TOKEN=false         # chỉ nhận token CMS (prod bắt buộc false)
```

`CMS_BASE_URL` dùng để ghép `avatarUrl` tương đối CMS trả về (`/api/avatars/68.png`).

## Bước 4 — Kiểm tra từng đường nối

```bash
make cms-check
```

Lệnh này tự đọc `.env` và kiểm 4 nhóm. Kết quả khi mọi thứ đúng:

```
1. JWKS cua CMS          ✓ JWKS doc duoc: cms-2026(RS256)
2. Workspace API         ✓ GET /groups OK — CMS tra 1 nhom
                           - wg_demo01  Nhom Thiet ke Web  (1 thanh vien)
3. Webhook Chat -> CMS   ✓ CMS nhan event va chap nhan chu ky — tra: {"ok":true}
4. Chat da bat tich hop  ✓ Chat song / Bot API da bat / chap nhan service key
```

Bảng lỗi (script in sẵn gợi ý):

| Báo lỗi | Nguyên nhân thường gặp |
|---|---|
| `JWKS khong goi duoc` | sai `CMS_WORKSPACE_BASE_URL`, hoặc firewall chặn Chat → CMS |
| `GET /groups -> 401` | `CHAT_SERVICE_KEY` hai bên khác nhau |
| `GET /groups -> 503` | **CMS** chưa cấu hình `CHAT_SERVICE_KEY` bên họ |
| `Webhook -> 401` | khoá lệch, hoặc **lệch giờ > 5 phút** giữa 2 máy (`timedatectl` / NTP) |
| `Webhook -> 404` | sai `webhookPath` — xin lại đúng path của bot |
| `Bot API chua bat` | quên `CMS_ENABLED=true` hoặc chưa khởi động lại Chat |

Script **không** kiểm được chiều CMS → Chat (phải do CMS gọi tới). Nhờ họ thử:

```bash
curl -i -X POST https://chat.cty.com/bot/v1/users/resolve \
  -H "X-Chat-Service-Key: $CHAT_SERVICE_KEY" -H 'Content-Type: application/json' \
  -d '{"items":[{"openId":"local:...","name":"Test"}]}'
```
Đúng thì trả `{"items":[{"openId":"…","actorId":"u_68","name":"…","presence":"off"}]}`.

## Bước 5 — Bật và xác nhận

```bash
make dev-down && make dev-up      # hoặc trên VPS: make prod-down && make prod-up
make dev-logs | head -30
```

Log phải thấy 2 dòng này:

```
msg="xac thuc bang JWKS cua CMS" url=... iss=designer-cms aud=abc-workspace
msg="dong bo nhom CMS" groups=<số nhóm>
```

Rồi kiểm bằng token thật của một người dùng CMS (đăng nhập ở CMS lấy `accessToken`):

```bash
curl -s https://chat.cty.com/api/client/v1/chat/bootstrap \
  -H "Authorization: Bearer <accessToken của CMS>" | jq '.element.me, .element.groups[0].chatId'
```

Thấy đúng tên người dùng + `wg_…` là **đã nối xong**.

## Bước 6 — Việc đổi ở phía FE

Chỉ **một** thứ: lấy token từ CMS (`POST /api/client/v1/workspace/auth/login` + `/auth/refresh`)
thay vì token dev. Đường dẫn và payload của Chat **không đổi** (xem [11-frontend.md](11-frontend.md)).

Ba thứ đổi phía sau lưng FE, không phải sửa code:
- nhóm trở thành chỉ đọc (CMS quản lý thành viên) → ẩn nút sửa thành viên khi `source: "cms"`;
- card do CMS dựng, có `visibleTo` — Chat đã lọc nút sẵn, FE render `card` + `actions` như cũ;
- `@bot yeu_cau` không còn tạo task nội bộ; yêu cầu tạo qua `POST /chat/forms/create_task/submit`.

## Ghi chú cho môi trường `designer-cms.memberappj.com` (đã dò thực tế)

| Mục | Giá trị đúng |
|---|---|
| JWKS | `https://designer-cms.memberappj.com/api/client/v1/workspace/.well-known/jwks.json` — OK, `kid=cms-20261118-5bebbf1f`, RS256 |
| Workspace API | `https://designer-cms.memberappj.com/api/client/v1/workspace` — OK với service key, trả nhóm `wg_00f2d7a7021bdf59` *TEAM TEST* (4 thành viên, tổ `DEV-TEST`) |
| Webhook | `https://designer-cms.memberappj.com/webhook/designer-order` — OK, trả `{"ok":true}`. **Lưu ý: không có đoạn `/web/`** như ví dụ dev trong tài liệu (`/webhook/web/designer`); đúng dạng là `/webhook/<webhookPath>` |

⚠️ `https://designer-cms.memberappj.com/api/webhook/...` trả **200 nhưng là trang HTML của SPA**, không
phải webhook. `make cms-check` đã được vá để báo lỗi trường hợp này thay vì coi là thành công.

Lấy `webhookPath`: vào CMS → **Bot** → bot của kênh Web → trường `webhookPath` (dev mẫu trong tài liệu
là `designer`, deployment này khác). Hoặc hỏi đội CMS: *"webhookPath của bot Web là gì?"*.

## Quay lui nếu có sự cố

```bash
CMS_ENABLED=false
TASK_ENGINE_ENABLED=true
AUTH_ALLOW_DEV_TOKEN=true    # chỉ ở dev/staging
```

Khởi động lại là về chế độ độc lập. **Dữ liệu không mất**: nhóm/tin nhắn/card đã lưu vẫn còn, chỉ
ngừng đồng bộ và ngừng gửi event sang CMS.
