# 12 — Deploy lên VPS

Trả lời ngắn cho câu "copy src lên VPS rồi `up` là được chứ?": **gần đúng** — code chạy được ngay,
nhưng `docker-compose.yml` (bản demo) **mở cổng Postgres/Redis ra Internet** và dùng mật khẩu mẫu.
Trên VPS hãy dùng `docker-compose.prod.yml`.

## 1. Khác nhau giữa 3 stack

| File | Dùng khi | Cổng mở ra ngoài | Bí mật |
|---|---|---|---|
| `docker-compose.dev.yml` | máy cá nhân, hot reload | 8080, 9090, **5432, 6379** | hardcode trong file |
| `docker-compose.yml` | thử bản build ở máy | 8080, 9090, **5432, 6379** | hardcode trong file |
| **`docker-compose.prod.yml`** | **VPS** | chỉ `127.0.0.1:8080`, `127.0.0.1:9090` | đọc từ `.env` |

## 2. Các bước trên VPS

```bash
# 1. Cài Docker + compose plugin, rồi lấy source
git clone <repo> /opt/chat-service && cd /opt/chat-service

# 2. Tạo .env từ mẫu rồi điền bí mật
cp .env.production.example .env
openssl rand -base64 48            # dùng cho JWT_SECRET
openssl rand -base64 24            # dùng cho POSTGRES_PASSWORD
nano .env

# 3. Chạy (tự build image, tự chạy migration rồi mới khởi động app)
make prod-up
make prod-logs
```

Kiểm tra: `curl -s localhost:8080/healthz` → `{"data":{"status":"ok"}}`, và
`curl -s localhost:8080/readyz` (ping được DB).

## 3. Đặt sau reverse proxy có TLS (bắt buộc)

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

## 4. Giai đoạn chưa có CMS

`APP_ENV=prod` **chặn token dev** (`AUTH_ALLOW_DEV_TOKEN` bắt buộc `false`), nên nếu muốn FE thử
trên VPS trước khi CMS sẵn sàng thì đặt `APP_ENV=staging` trong `.env`:

```bash
APP_ENV=staging
AUTH_ALLOW_DEV_TOKEN=true
JWT_SECRET=<secret thật, không dùng giá trị mẫu>
```

Khi CMS xong: `APP_ENV=prod`, `AUTH_ALLOW_DEV_TOKEN=false`, `CMS_ENABLED=true`, điền
`CHAT_SERVICE_KEY` + các `CMS_*` (xem [10-cms-integration.md](10-cms-integration.md)).

## 5. Service tự kiểm tra cấu hình

Khi `APP_ENV=prod`, service **không khởi động** nếu gặp:

```
config khong an toan cho prod:
  - AUTH_ALLOW_DEV_TOKEN=true o prod: token HS256 tu ky se dang nhap duoc bang bat ky ai
  - JWT_SECRET dang dung gia tri mau trong .env.example
  - POSTGRES_AUTO_MIGRATE=true o prod: phai chay migration bang golang-migrate
```

Và cảnh báo (vẫn chạy): `HTTP_ALLOW_ORIGINS=*`, `GRPC_REFLECTION=true`, `LOG_LEVEL=debug`,
`POSTGRES_LOG_SQL=true`.

## 6. Vận hành

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

## 7. Chạy nhiều instance

`REDIS_ENABLED=true` (prod compose bật sẵn) nên scale ngang được mà không cần sticky session:
bỏ `ports` cố định của app, thêm `--scale app=3` và để nginx cân tải. Mỗi instance tự nhận event
qua Redis — đã kiểm chứng ở [01-architecture.md](01-architecture.md) §3.

## 8. Ba cái bẫy gặp thật khi thử stack này

| Hiện tượng | Nguyên nhân / cách xử lý |
|---|---|
| `password authentication failed for user "..."` dù `.env` đúng | Postgres chỉ tạo user/mật khẩu ở **lần chạy đầu trên volume rỗng**. Đổi `POSTGRES_PASSWORD` sau đó không có tác dụng. Prod dùng volume riêng `pgdata-prod`; nếu cần đổi mật khẩu: `ALTER USER ... PASSWORD ...` trong psql, hoặc xoá volume (mất dữ liệu). |
| `docker compose up -d` treo rất lâu | `migrate` lỗi và bị restart lặp, app chờ `service_completed_successfully`. Prod compose đặt `restart: "no"` cho migrate để lỗi hiện ra ngay — xem `docker compose -f docker-compose.prod.yml logs migrate`. |
| FE gọi được REST nhưng WebSocket rớt liên tục | reverse proxy thiếu header `Upgrade`/`Connection` hoặc `proxy_read_timeout` ngắn hơn chu kỳ ping (25s). |
| Chạy stack prod xong thì stack dev chết với `stat /app/server: no such file` | Ba compose file nằm cùng thư mục nên mặc định **chung project name** → build image của stack này ghi đè stack kia. Đã đặt `name:` riêng cho từng file (`chat-service-dev` / `-demo` / `-prod`). Nếu đang có container cũ từ trước: `docker rm -f chat-postgres-dev chat-redis-dev chat-app-dev` rồi `make dev-up` lại. |

## 9. Checklist trước khi mở cho người dùng

- [ ] Dùng `docker-compose.prod.yml`, **không** dùng file demo.
- [ ] `.env` có `JWT_SECRET` / `POSTGRES_PASSWORD` sinh ngẫu nhiên; `.env` không được commit (đã có trong `.gitignore`).
- [ ] `APP_ENV=prod` (hoặc `staging` nếu còn dùng token dev) — service sẽ tự chặn cấu hình nguy hiểm.
- [ ] `HTTP_ALLOW_ORIGINS` là domain FE, không để `*`.
- [ ] `GRPC_REFLECTION=false`.
- [ ] TLS + reverse proxy đã cấu hình WebSocket; firewall chỉ mở 22/80/443.
- [ ] Đã chạy migration (`make prod-up` tự chạy) và `curl /readyz` trả 200.
- [ ] Có cron backup database và đã thử khôi phục một lần.
