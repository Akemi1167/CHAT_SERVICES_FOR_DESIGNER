# 08 — Chạy bằng Docker

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

## 1. Vòng lặp phát triển (khuyên dùng)

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

## 2. Chạy bản build (giống prod)

```bash
make up            # build image + chạy postgres/redis/migrate/app
make logs
make down
```

Image runtime: `alpine` + binary tĩnh, chạy bằng user `app` (uid 10001), có `HEALTHCHECK` gọi `/healthz`.

## 3. Biến môi trường trong Docker

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

### Console sạch, chỉ in cái mình cần

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

### Mật khẩu Redis

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

Prod thì **bắt buộc** — xem [12-deploy.md](12-deploy.md) §7.

**Trước khi deploy thật** đổi `JWT_SECRET`, đặt `REDIS_PASSWORD`, đặt `HTTP_ALLOW_ORIGINS`
theo domain FE, `GRPC_REFLECTION=false`, `APP_ENV=prod`, và `POSTGRES_AUTO_MIGRATE=false`
(mặc định đã false).

## 4. Sự cố thường gặp

| Hiện tượng | Nguyên nhân / cách xử lý |
|---|---|
| `bind: address already in use` | cổng 5432/6379/8080/9090 đã bị tiến trình khác giữ — đổi port map trong compose |
| app restart liên tục, log `config: thieu bien moi truong bat buoc` | thiếu `POSTGRES_DSN` hoặc `JWT_SECRET` trong env của service `app` |
| `Dirty database version` khi migrate | migration lỗi giữa chừng: `make migrate-force version=<n>` rồi chạy lại |
| Sửa `.env` nhưng container vẫn dùng giá trị cũ | `docker compose restart` **không nạp lại** `env_file`. Phải `docker compose -f docker-compose.dev.yml up -d app` (tạo lại container). Kiểm tra: `docker compose -f docker-compose.dev.yml exec app sh -c 'echo $CMS_WEBHOOK_URL'` |
| air không build lại | file sửa nằm trong `exclude_dir` của `.air.toml`; hoặc thư mục `tmp/` bị xoá — **`tmp/` là chỗ air build binary vào, đừng dùng nó làm chỗ chứa file tạm**. Script/probe tạm thì để trong `scratch/` (đã được air bỏ qua). Khắc phục: `docker compose -f docker-compose.dev.yml restart app` |
| đổi `.proto` xong container không thấy | chạy `make dev-sh` → `make proto` (image dev đã có `protoc`) |
