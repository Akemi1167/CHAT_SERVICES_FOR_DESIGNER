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

**Trước khi deploy thật** đổi `JWT_SECRET`, đặt `HTTP_ALLOW_ORIGINS` theo domain FE,
`GRPC_REFLECTION=false`, `APP_ENV=prod`, và `POSTGRES_AUTO_MIGRATE=false` (mặc định đã false).

## 4. Sự cố thường gặp

| Hiện tượng | Nguyên nhân / cách xử lý |
|---|---|
| `bind: address already in use` | cổng 5432/6379/8080/9090 đã bị tiến trình khác giữ — đổi port map trong compose |
| app restart liên tục, log `config: thieu bien moi truong bat buoc` | thiếu `POSTGRES_DSN` hoặc `JWT_SECRET` trong env của service `app` |
| `Dirty database version` khi migrate | migration lỗi giữa chừng: `make migrate-force version=<n>` rồi chạy lại |
| air không build lại | file sửa nằm trong `exclude_dir` của `.air.toml`; hoặc thư mục `tmp/` bị xoá — **`tmp/` là chỗ air build binary vào, đừng dùng nó làm chỗ chứa file tạm**. Script/probe tạm thì để trong `scratch/` (đã được air bỏ qua). Khắc phục: `docker compose -f docker-compose.dev.yml restart app` |
| đổi `.proto` xong container không thấy | chạy `make dev-sh` → `make proto` (image dev đã có `protoc`) |
