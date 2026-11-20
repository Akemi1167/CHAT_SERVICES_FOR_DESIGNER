# Deploy — Docker, chạy thật, cutover CMS

Chạy service bằng Docker ở local, đưa lên server, và các bước bật tích hợp CMS.

## Nội dung

- [Docker](#docker)
- [Chạy thật (production)](#chạy-thật-production)
- [Cutover: bật tích hợp CMS](#cutover-bật-tích-hợp-cms)

---

## Docker

Có **2 stack**, chọn 1:

| | `docker-compose.dev.yml` (DEV) | `docker-compose.yml` (prod-like) |
|---|---|---|
| Lệnh | `make dev-up` | `make up` |
| Source | **mount vào container**, `air` tự build lại khi sửa code | build sẵn vào image, không mount |
| Dockerfile | `Dockerfile.dev` (có Go toolchain, air, protoc) | `Dockerfile` (multi-stage → alpine ~60MB, non-root) |
| Dùng khi | đang code hằng ngày | kiểm thử bản build, CI, deploy |
| Container | `chat-postgres-dev`, `chat-redis-dev`, `chat-app-dev` | `chat-postgres`, `chat-redis`, `chat-app` |
| Volume | `pgdata-dev` + cache `go-mod-cache`, `go-build-cache` | `pgdata` |

Cả hai stack đều chạy `migrate/migrate` trước khi app lên (`depends_on: service_completed_successfully`),
nên schema luôn đúng trước khi service khởi động.

### 1. Vòng lặp phát triển (khuyên dùng)

```bash
make dev-up        # lần đầu ~1-2 phút (build image + tải module), sau đó nhanh
make dev-logs      # thấy log air: "building..." -> "running..."
```

Sửa bất kỳ file `.go` → air build lại trong container (~3-5s) và chạy lại service. Không cần cài Go
trên máy. Postgres mở ở `localhost:5432`, Redis `localhost:6379`, API `localhost:8080`, gRPC `localhost:9090`.

Lệnh phụ trợ:

```bash
make dev-test      # go test ./... bên trong container
make dev-sh        # vào shell container (chạy make proto, go mod tidy... bên trong)
make dev-migrate   # chạy lại migration
make dev-psql      # mở psql
make dev-down      # dừng (giữ dữ liệu)
make dev-down-v    # dừng + xoá volume (mất dữ liệu + cache)
```

Lưu ý khi thêm dependency: chạy `make dev-sh` rồi `go mod tidy` bên trong (hoặc `make deps` ở máy host),
module cache nằm ở volume nên không phải tải lại.

### 2. Chạy bản build (giống prod)

```bash
make up            # build image + chạy postgres/redis/migrate/app
make logs
make down
```

Image runtime: `alpine` + binary tĩnh, chạy bằng user `app` (uid 10001), có `HEALTHCHECK` gọi `/healthz`.

### 3. Biến môi trường trong Docker

Hai compose file set env trực tiếp cho dịch vụ `app` (xem trong file). Khác biệt so với chạy ở máy:

```yaml
POSTGRES_DSN: "postgres://chat:chat@postgres:5432/chat?sslmode=disable"  # host = tên service
REDIS_ENABLED: "true"
REDIS_ADDR:    "redis:6379"
```

`REDIS_ENABLED=true` để event WebSocket fanout qua Redis — chạy nhiều instance được ngay:

```bash
docker compose -f docker-compose.dev.yml up -d --scale app=2   # cần bỏ container_name + cố định port trước
```

#### Console sạch, chỉ in cái mình cần

Mặc định mọi log JSON/text đổ ra stdout nên `make dev-logs` rất ồn. Hai biến trong `.env`:

```bash
LOG_FILE=./logs/app.log    # log đi đâu
LOG_STDOUT=false           # console KHÔNG in log nữa (phải có LOG_FILE)
```

Khi `LOG_STDOUT=false`, app in ra console đúng **một** dòng để biết log nằm đâu:

```
log -> ./logs/app.log (LOG_STDOUT=false, console nay chi con SQL log neu bat POSTGRES_LOG_SQL)
```

Sau đó console trống. Cần xem SQL thì bật lên, xong tắt đi:

```bash
make sql-on      # sửa POSTGRES_LOG_SQL trong .env rồi nạp lại app (~2s)
make dev-logs    # giờ chỉ còn câu SQL, có màu
make sql-off
```

```
┌─ SQL  0.21ms  rows=4  repository/postgres/conversation_repo.go:44
│ SELECT *
│ FROM "conversation_members"
│ WHERE "conversation_members"."conversation_id" = 'b6fff02b-…'
│   AND left_at IS NULL
└─
```

Log đầy đủ vẫn nằm trong file: `make log-file`, `make log-grep Q=groups/refresh`.

Ba chi tiết khiến nó chạy được:

- `tty: true` cho service `app` — không có TTY thì SQL log mất màu.
- `logs` nằm trong `exclude_dir` của `.air.toml` — nếu không, air thấy file log đổi là build lại.
- Các công tắc debug trong compose viết dạng `"${POSTGRES_LOG_SQL:-false}"`.
  **`environment:` với giá trị cố định sẽ đè lên `env_file`** — đó là lý do trước đây sửa
  `.env` mà không ăn gì.

Prod thì ngược lại: `LOG_STDOUT=true`, `LOG_FILE` để trống. `WarnProd()` sẽ cảnh báo nếu sai.

#### Mật khẩu Redis

`REDIS_PASSWORD` trong `.env` điều khiển **cả hai phía** cùng lúc:

| Để trống (mặc định local) | Có giá trị |
|---|---|
| `redis-server --appendonly no` | `redis-server --appendonly no --requirepass <pass>` |
| app kết nối không mật khẩu | app gửi mật khẩu khi kết nối |

Compose làm được việc này nhờ shell trong container tự chọn tham số:

```yaml
command:
  - sh
  - -c
  - exec redis-server --appendonly no $${REDIS_PASSWORD:+--requirepass "$$REDIS_PASSWORD"}
```

`$$` để compose **không** nuốt mất biến — nó phải tới tay `sh` bên trong container, vì
chỉ ở đó mới biết biến rỗng hay không. Biến rỗng thì `${VAR:+...}` cho ra chuỗi rỗng,
lệnh y hệt như cũ.

Đặt mật khẩu ở local:

```bash
openssl rand -hex 16                       # dán vào REDIS_PASSWORD trong .env
docker compose up -d --force-recreate redis    # `restart` KHÔNG nạp lại command/env
make redis-cli                             # tự dùng mật khẩu trong .env
```

Kiểm tra đã bật: `docker compose exec -T redis redis-cli ping` (không mật khẩu) phải trả
`NOAUTH Authentication required.`

Prod thì **bắt buộc** — xem [Chạy thật (production)](#chạy-thật-production) §7.

**Trước khi deploy thật** đổi `JWT_SECRET`, đặt `REDIS_PASSWORD`, đặt `HTTP_ALLOW_ORIGINS`
theo domain FE, `GRPC_REFLECTION=false`, `APP_ENV=prod`, và `POSTGRES_AUTO_MIGRATE=false`
(mặc định đã false).

### 4. Sự cố thường gặp

| Hiện tượng | Nguyên nhân / cách xử lý |
|---|---|
| `bind: address already in use` | cổng 5432/6379/8080/9090 đã bị tiến trình khác giữ — đổi port map trong compose |
| app restart liên tục, log `config: thieu bien moi truong bat buoc` | thiếu `POSTGRES_DSN` hoặc `JWT_SECRET` trong env của service `app` |
| `Dirty database version` khi migrate | migration lỗi giữa chừng: `make migrate-force version=<n>` rồi chạy lại |
| Sửa `.env` nhưng container vẫn dùng giá trị cũ | `docker compose restart` **không nạp lại** `env_file`. Phải `docker compose -f docker-compose.dev.yml up -d app` (tạo lại container). Kiểm tra: `docker compose -f docker-compose.dev.yml exec app sh -c 'echo $CMS_WEBHOOK_URL'` |
| air không build lại | file sửa nằm trong `exclude_dir` của `.air.toml`; hoặc thư mục `tmp/` bị xoá — **`tmp/` là chỗ air build binary vào, đừng dùng nó làm chỗ chứa file tạm**. Script/probe tạm thì để trong `scratch/` (đã được air bỏ qua). Khắc phục: `docker compose -f docker-compose.dev.yml restart app` |
| đổi `.proto` xong container không thấy | chạy `make dev-sh` → `make proto` (image dev đã có `protoc`) |

---

## Chạy thật (production)

Trả lời ngắn cho câu "copy src lên VPS rồi `up` là được chứ?": **gần đúng** — code chạy được ngay,
nhưng `docker-compose.yml` (bản demo) **mở cổng Postgres/Redis ra Internet** và dùng mật khẩu mẫu.
Trên VPS hãy dùng `docker-compose.prod.yml`.

### 1. Khác nhau giữa 3 stack

| File | Dùng khi | Cổng mở ra ngoài | Bí mật |
|---|---|---|---|
| `docker-compose.dev.yml` | máy cá nhân, hot reload | 8080, 9090, **5432, 6379** | hardcode trong file |
| `docker-compose.yml` | thử bản build ở máy | 8080, 9090, **5432, 6379** | hardcode trong file |
| **`docker-compose.prod.yml`** | **VPS** | chỉ `127.0.0.1:8080`, `127.0.0.1:9090` | đọc từ `.env` |

### 2. Các bước trên VPS

```bash
# 1. Cài Docker + compose plugin, rồi lấy source
git clone <repo> /opt/chat-service && cd /opt/chat-service

# 2. Tạo .env từ mẫu rồi điền bí mật
cp .env.production.example .env
openssl rand -base64 48            # dùng cho JWT_SECRET
openssl rand -base64 24            # dùng cho POSTGRES_PASSWORD
openssl rand -base64 24            # dùng cho REDIS_PASSWORD
nano .env

# 3. Chạy (tự build image, tự chạy migration rồi mới khởi động app)
make prod-up
make prod-logs
```

Kiểm tra: `curl -s localhost:8080/healthz` → `{"data":{"status":"ok"}}`, và
`curl -s localhost:8080/readyz` (ping được DB).

### 3. Đặt sau reverse proxy có TLS (bắt buộc)

App chỉ lắng nghe `127.0.0.1` nên phải có nginx/Caddy ở host. **WebSocket cần header Upgrade và
timeout dài hơn chu kỳ ping (25s)** — thiếu là kết nối bị cắt mỗi phút.

```nginx
server {
    listen 443 ssl http2;
    server_name chat.cong-ty.com;

    ssl_certificate     /etc/letsencrypt/live/chat.cong-ty.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/chat.cong-ty.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade    $http_upgrade;      # WebSocket
        proxy_set_header Connection "upgrade";
        proxy_set_header Host       $host;
        proxy_set_header X-Real-IP  $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout  300s;                        # > WS_PING_INTERVAL
        proxy_send_timeout  300s;
    }
}
```

Caddy thì ngắn hơn và tự lo chứng chỉ:

```
chat.cong-ty.com {
    reverse_proxy 127.0.0.1:8080
}
```

FE kết nối `wss://chat.cong-ty.com/ws?token=…` (không phải `ws://`).

Tường lửa: chỉ mở 22, 80, 443. **Không** mở 5432/6379/8080/9090.

### 4. Giai đoạn chưa có CMS

`APP_ENV=prod` **chặn token dev** (`AUTH_ALLOW_DEV_TOKEN` bắt buộc `false`), nên nếu muốn FE thử
trên VPS trước khi CMS sẵn sàng thì đặt `APP_ENV=staging` trong `.env`:

```bash
APP_ENV=staging
AUTH_ALLOW_DEV_TOKEN=true
JWT_SECRET=<secret thật, không dùng giá trị mẫu>
```

Khi CMS xong: `APP_ENV=prod`, `AUTH_ALLOW_DEV_TOKEN=false`, `CMS_ENABLED=true`, điền
`CHAT_SERVICE_KEY` + các `CMS_*` (xem [Tích hợp CMS (Designer Bot)](backend.md#tích-hợp-cms-designer-bot)).

### 5. Service tự kiểm tra cấu hình

Khi `APP_ENV=prod`, service **không khởi động** nếu gặp:

```
config khong an toan cho prod:
  - AUTH_ALLOW_DEV_TOKEN=true o prod: token HS256 tu ky se dang nhap duoc bang bat ky ai
  - JWT_SECRET dang dung gia tri mau trong .env.example
  - POSTGRES_AUTO_MIGRATE=true o prod: phai chay migration bang golang-migrate
```

Và cảnh báo (vẫn chạy): `HTTP_ALLOW_ORIGINS=*`, `GRPC_REFLECTION=true`, `LOG_LEVEL=debug`,
`POSTGRES_LOG_SQL=true`.

### 6. Vận hành

```bash
make prod-logs                       # xem log (JSON, có request_id)
make prod-migrate                    # chạy lại migration
make prod-backup OUT=/backup/chat-$(date +%F).sql
make prod-down                       # dừng, giữ dữ liệu
docker compose -f docker-compose.prod.yml up -d --build   # cập nhật phiên bản mới
```

Backup định kỳ bằng cron:

```cron
0 2 * * * cd /opt/chat-service && make prod-backup OUT=/backup/chat-$(date +\%F).sql
```

Khôi phục: `docker compose -f docker-compose.prod.yml exec -T postgres psql -U <user> -d <db> < backup.sql`

### 7. Chạy nhiều instance

`REDIS_ENABLED=true` (prod compose bật sẵn) nên scale ngang được mà không cần sticky session:
bỏ `ports` cố định của app, thêm `--scale app=3` và để nginx cân tải. Mỗi instance tự nhận event
qua Redis — đã kiểm chứng ở [Kiến trúc](source.md#kiến-trúc) §3.

**Presence khi chạy nhiều instance**: mỗi pod tự làm tươi `last_seen_at` cho những người nối
vào chính nó (mỗi `PRESENCE_HEARTBEAT`, mặc định 30s), và mọi bề mặt coi `online` là hết hạn sau
`PRESENCE_TTL` (90s). Không cần pod nào biết pod khác, cũng không cần registry riêng. Pod chết thì
người dùng của nó tự chuyển `offline` sau 90 giây.

Đổi `PRESENCE_HEARTBEAT` thì đổi `PRESENCE_TTL` theo, giữ tỉ lệ ít nhất 1:3.

**Redis phải có mật khẩu.** `REDIS_PASSWORD` dùng cho cả hai phía: compose truyền vào
`redis-server --requirepass`, và app dùng để kết nối. Prod bắt buộc — thiếu biến thì
`make prod-up` dừng ngay với thông báo `required variable REDIS_PASSWORD is missing a value`,
app không khởi động với Redis trần.

```bash
openssl rand -base64 24    # đặt vào REDIS_PASSWORD trong .env
make prod-up
```

Sai mật khẩu thì app chết lúc boot chứ không chạy ngầm rồi hỏng lặng lẽ:

```
bootstrap error: app: redis ping: WRONGPASS invalid username-password pair or user is disabled.
```

Prod compose **không** publish cổng 6379 ra host — Redis chỉ nghe trong network docker.
Mật khẩu là lớp thứ hai, phòng khi có container khác bị chiếm trong cùng network.

### 8. Ba cái bẫy gặp thật khi thử stack này

| Hiện tượng | Nguyên nhân / cách xử lý |
|---|---|
| `password authentication failed for user "..."` dù `.env` đúng | Postgres chỉ tạo user/mật khẩu ở **lần chạy đầu trên volume rỗng**. Đổi `POSTGRES_PASSWORD` sau đó không có tác dụng. Prod dùng volume riêng `pgdata-prod`; nếu cần đổi mật khẩu: `ALTER USER ... PASSWORD ...` trong psql, hoặc xoá volume (mất dữ liệu). |
| `docker compose up -d` treo rất lâu | `migrate` lỗi và bị restart lặp, app chờ `service_completed_successfully`. Prod compose đặt `restart: "no"` cho migrate để lỗi hiện ra ngay — xem `docker compose -f docker-compose.prod.yml logs migrate`. |
| FE gọi được REST nhưng WebSocket rớt liên tục | reverse proxy thiếu header `Upgrade`/`Connection` hoặc `proxy_read_timeout` ngắn hơn chu kỳ ping (25s). |
| Chạy stack prod xong thì stack dev chết với `stat /app/server: no such file` | Ba compose file nằm cùng thư mục nên mặc định **chung project name** → build image của stack này ghi đè stack kia. Đã đặt `name:` riêng cho từng file (`chat-service-dev` / `-demo` / `-prod`). Nếu đang có container cũ từ trước: `docker rm -f chat-postgres-dev chat-redis-dev chat-app-dev` rồi `make dev-up` lại. |

### 9. Checklist trước khi mở cho người dùng

- [ ] Dùng `docker-compose.prod.yml`, **không** dùng file demo.
- [ ] `.env` có `JWT_SECRET` / `POSTGRES_PASSWORD` sinh ngẫu nhiên; `.env` không được commit (đã có trong `.gitignore`).
- [ ] `APP_ENV=prod` (hoặc `staging` nếu còn dùng token dev) — service sẽ tự chặn cấu hình nguy hiểm.
- [ ] `HTTP_ALLOW_ORIGINS` là domain FE, không để `*`.
- [ ] `GRPC_REFLECTION=false`.
- [ ] TLS + reverse proxy đã cấu hình WebSocket; firewall chỉ mở 22/80/443.
- [ ] Đã chạy migration (`make prod-up` tự chạy) và `curl /readyz` trả 200.
- [ ] Có cron backup database và đã thử khôi phục một lần.

---

## Cutover: bật tích hợp CMS

Làm theo đúng thứ tự: **xin thông tin → đưa thông tin → điền env → `make cms-check` → bật**.
Nền tảng kỹ thuật ở [Tích hợp CMS (Designer Bot)](backend.md#tích-hợp-cms-designer-bot); file này chỉ là quy trình cắm dây.

### Bước 1 — Xin đội CMS 4 giá trị

| Cần | Là gì | Ví dụ | Hỏi thế nào |
|---|---|---|---|
| `CHAT_SERVICE_KEY` | **khoá dùng chung** cho cả 3 hướng (ký webhook, Chat gọi Workspace API, CMS gọi Bot API) | `9f2c…` | "Cho mình giá trị `CHAT_SERVICE_KEY` trong `.env` của CMS" |
| Base URL Workspace API | nơi Chat đọc nhóm/user | `https://cms.cty.com/api/client/v1/workspace` | "Base của Workspace API trên môi trường dev/prod?" |
| URL webhook của bot | nơi Chat gửi event | `https://cms.cty.com/webhook/web/designer` | "`webhookPath` của bot Web là gì?" |
| `iss` / `aud` của token | để Chat kiểm token người dùng | `designer-cms` / `abc-workspace` | "Token các bạn phát có `iss`/`aud` là gì?" (mặc định đúng như ví dụ) |

Hỏi thêm cho chắc: **CMS gọi Bot API của Chat bằng domain nào** — cần mở firewall cho IP đó.

### Bước 2 — Đưa đội CMS 2 giá trị

| Đưa | Giá trị | Họ dùng để |
|---|---|---|
| `CHAT_API_BASE` | `https://chat.cty.com/bot/v1` | gọi Bot API của Chat (gửi card, thông báo, đẩy nhóm) |
| `CHAT_SERVICE_KEY` | **cùng một khoá** ở bước 1 | gắn header `X-Chat-Service-Key` |

Nói rõ với họ: Chat đã làm đủ 10 endpoint Bot API (`users/resolve`, `messages`, `PATCH messages/:id`,
`messages/:id/thread`, `messages/:id/pin`, `notifications`, `presence`, `groups/:id`,
`groups/:id/members`, `attachments/:id`), có xử lý `Idempotency-Key`, và lỗi trả dạng
`{code, message, retryable}`.

### Bước 3 — Điền `.env`

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

### Bước 4 — Kiểm tra từng đường nối

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

### Bước 5 — Bật và xác nhận

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

### Bước 6 — Việc đổi ở phía FE

Chỉ **một** thứ: lấy token từ CMS (`POST /api/client/v1/workspace/auth/login` + `/auth/refresh`)
thay vì token dev. Đường dẫn và payload của Chat **không đổi** (xem [Hợp đồng FE](frontend.md#hợp-đồng-fe)).

Ba thứ đổi phía sau lưng FE, không phải sửa code:
- nhóm trở thành chỉ đọc (CMS quản lý thành viên) → ẩn nút sửa thành viên khi `source: "cms"`;
- card do CMS dựng, có `visibleTo` — Chat đã lọc nút sẵn, FE render `card` + `actions` như cũ;
- `@bot yeu_cau` không còn tạo task nội bộ; yêu cầu tạo qua `POST /chat/forms/create_task/submit`.

### Ghi chú cho môi trường `designer-cms.memberappj.com` (đã dò thực tế)

| Mục | Giá trị đúng |
|---|---|
| JWKS | `https://designer-cms.memberappj.com/api/client/v1/workspace/.well-known/jwks.json` — OK, `kid=cms-20261118-5bebbf1f`, RS256 |
| Workspace API | `https://designer-cms.memberappj.com/api/client/v1/workspace` — OK với service key, trả nhóm `wg_00f2d7a7021bdf59` *TEAM TEST* (4 thành viên, tổ `DEV-TEST`) |
| Webhook | `https://designer-cms.memberappj.com/webhook/designer-order` — OK, trả `{"ok":true}`. **Lưu ý: không có đoạn `/web/`** như ví dụ dev trong tài liệu (`/webhook/web/designer`); đúng dạng là `/webhook/<webhookPath>` |

⚠️ `https://designer-cms.memberappj.com/api/webhook/...` trả **200 nhưng là trang HTML của SPA**, không
phải webhook. `make cms-check` đã được vá để báo lỗi trường hợp này thay vì coi là thành công.

Lấy `webhookPath`: vào CMS → **Bot** → bot của kênh Web → trường `webhookPath` (dev mẫu trong tài liệu
là `designer`, deployment này khác). Hoặc hỏi đội CMS: *"webhookPath của bot Web là gì?"*.

### Quay lui nếu có sự cố

```bash
CMS_ENABLED=false
TASK_ENGINE_ENABLED=true
AUTH_ALLOW_DEV_TOKEN=true    # chỉ ở dev/staging
```

Khởi động lại là về chế độ độc lập. **Dữ liệu không mất**: nhóm/tin nhắn/card đã lưu vẫn còn, chỉ
ngừng đồng bộ và ngừng gửi event sang CMS.
