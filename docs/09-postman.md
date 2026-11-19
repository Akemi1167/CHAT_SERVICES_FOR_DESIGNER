# 09 — Postman

Thư mục `internal/postman/` chỉ có **2 file**:

| File | Dùng để |
|---|---|
| `chat-service.postman_collection.json` | Import vào app Postman (Import → File) |
| `chat-service.local.postman_environment.json` | Environment đi kèm |

Cả hai sinh từ `scripts/gen_postman.py` (`make postman`). **Sửa API thì sửa script rồi chạy
lại, đừng sửa tay file JSON** — `make postman-lint` sẽ báo nếu file trên đĩa lệch với script.

> Trước đây còn một cây `internal/postman/collections/` định dạng v3 (mỗi request một file
> YAML) cho Postman CLI. Đã **xoá**: hai định dạng song song cho cùng một bộ API là trùng lặp.
> Mẫu WebSocket/gRPC — thứ duy nhất v2.1 không chứa được — giờ nằm trong folder
> **`10 · WebSocket & gRPC (mẫu frame)`** của chính file JSON, đọc ở tab Documentation.

## 1. Import và dùng (3 bước)

1. Chạy service: `make dev-up` (hoặc `make up-deps && make migrate-up && make run`).
2. Postman → **Import** → chọn 2 file:
   `internal/postman/chat-service.postman_collection.json` và `internal/postman/chat-service.local.postman_environment.json`.
3. Chọn environment **Chat Service · Local**, lấy token dev rồi dán vào biến `token`:

```bash
make token SUB=user-001 NAME="Nguyen Van A" ROLE=designer
```

**Mới dùng thì chạy folder `FLOW` trước** — 16 bước đánh số theo đúng thứ tự thật của hệ
thống (xem mục 3). Muốn tra cứu từng endpoint thì dùng các folder `00` → `09`.

Mỗi request tự lưu id cần thiết vào collection variable (`user_id`, `group_id`, `message_id`,
`thread_id`, `team_id`, `task_id`, `task_code`, `assignment_id`, `revision_id`) nên bước sau
dùng được ngay, không phải copy id bằng tay.

Muốn thử phân công 2 người (chia task, đổi người phụ trách, chat riêng): tạo token thứ 2
(`make token SUB=user-002 ROLE=order`), gọi `GET /api/v1/me` bằng token đó, rồi dán `user_id`
nhận được vào biến `user_id_2`.

## 2. Ba định danh dễ nhầm

Trước đây collection có `conversation_id`, `chat_id` và `team_id`. Hai cái đầu là **cùng một
nhóm chat**, chỉ khác dạng id — đã gộp thành **`group_id`**:

| Biến | Là gì | Dạng giá trị | Dùng ở |
|---|---|---|---|
| `group_id` | **nhóm chat** | UUID nội bộ **hoặc** `wg_<hex>` | cả 3 bề mặt: `/api/v1`, `/bot/v1`, `/api/client/v1` |
| `cms_chat_id` | chatId mà **CMS** muốn tạo | bắt buộc `wg_<hex>` | chỉ `PUT /bot/v1/groups/:id` |
| `team_id` | **tổ** của module task nội bộ | UUID | `/api/v1/teams/*` |

- `group_id` dùng được ở mọi nơi vì route hội thoại của `/api/v1` nhận **cả hai dạng id**
  (`convIDParam` trong `internal/transport/httpserver/handler/conversation.go`): là UUID thì
  dùng luôn, không phải UUID thì tra `conversations.external_id`.
- `cms_chat_id` phải tách riêng vì tham số URL của `PUT /bot/v1/groups/:id` **chính là** chatId
  cần tạo — truyền UUID vào sẽ đẻ ra một nhóm có `external_id` là chuỗi UUID đó.
- `team_id` là **thực thể khác hẳn** (bảng `teams`), không gộp được. Cũng đừng nhầm với `teamId`
  trong body `POST /forms/create_task/submit` — đó là **tổ của CMS**, dạng số (`"27"`), Chat
  chuyển nguyên sang CMS chứ không diễn giải.

Hệ quả: `GET /api/v1/conversations/<chuỗi-bất-kỳ>` không còn trả `400 invalid_uuid` mà trả
`404 conversation_not_found` — vì chuỗi đó giờ được hiểu là một `external_id` có thể có.

## 3. Folder FLOW — chạy từ trên xuống để hiểu luồng

16 bước đánh số; hai bước **cùng cấp** thì `1.1`, `1.2`…

| Giai đoạn | Bước |
|---|---|
| 1 · chuẩn bị | `1.1` healthz · `1.2` `/me` (lưu `user_id`) |
| 2 · nhóm & thành viên | `2.1` tạo nhóm (lưu `group_id`) · `2.2` CMS đẩy nhóm sang · `2.3` xem thành viên (lưu `actor_id`) · `2.4` bootstrap của FE |
| 3 · tin nhắn | `3.1` gửi · `3.2` trả lời thread · `3.3` đọc lịch sử · `3.4` đánh dấu đã đọc |
| 4 · card & form | `4.1` CMS gửi card · `4.2` FE gửi form sang CMS |
| 5 · thông báo | `5.1` CMS tạo · `5.2` xem chưa đọc · `5.3` đánh dấu đã đọc |
| 6 · dọn dẹp | `6.1` xoá tin nhắn |

**Dừng ở một bước để xem kỹ**: đặt biến `stop_at` = số bước rồi Run collection.

```bash
make postman-flow TOKEN=$(make -s token SUB=demo-an) KEY=$CHAT_SERVICE_KEY STOP=2.3
# chạy 1.1 → 2.3 rồi dừng, in: "DUNG tai buoc 2.3 (stop_at). Doi stop_at de di tiep."
```

Để trống `STOP` thì chạy hết 16 bước. Bước `2.2` cố ý **không** đổi `group_id`: nhóm CMS đẩy
sang chỉ có thành viên do CMS khai, token của bạn không ở trong đó nên các bước `3.x` sẽ 403.

## 4. Nội dung collection (78 request / 9 folder)

| Folder | Nội dung |
|---|---|
| `FLOW` | 16 bước đánh số, chạy tuần tự — xem mục 3 |
| `00 · Bắt đầu` | healthz, readyz, `/me` (lưu `user_id`) |
| `01 · Hội thoại` | tạo nhóm / chat riêng, danh sách, chi tiết, thêm thành viên |
| `02 · Tin nhắn` | gửi text / ảnh / trong thread, lịch sử, tìm kiếm, đã đọc, ai đã xem, xoá |
| `04 · Bot @bot & gửi mẫu` | schema mẫu, `@bot` help, `@bot yeu_cau`, submit mẫu từ form, mẫu đã lưu |
| `07 · Lỗi & quyền` | 401 thiếu token, 404 id nhóm không có, 403 không phải thành viên, 404 route sai, 400 tin nhắn rỗng |
| `99 · Dọn dẹp` | rời nhóm / xoá thành viên (chạy sau cùng) |
| `08 · CMS → Chat (Bot API)` | CMS gọi sang Chat: resolve user, gửi/sửa card, thread, thông báo, đẩy nhóm |
| `09 · API cho FE (hợp đồng CMS)` | `/api/client/v1/chat/*`: bootstrap, tin nhắn, bấm nút card, gửi form, thông báo |

Mỗi folder cần cấu hình khác nhau:

| Folder | Cần gì |
|---|---|
| `FLOW`, `00`–`04`, `07`, `99` | chạy với cấu hình mặc định |
| `08`, `09` (CMS) | `CMS_ENABLED=true` + biến `service_key`, `cms_token`, `group_id` — xem [10-cms-integration.md](10-cms-integration.md) |

```bash
make postman-flow TOKEN=$(make -s token) KEY=$CHAT_SERVICE_KEY [STOP=2.3]   # folder FLOW
make postman-run TOKEN=$(make -s token)                 # tất cả trừ folder CMS
make postman-run-cms TOKEN=<access token CMS> KEY=<CHAT_SERVICE_KEY> CHAT_ID=wg_...
```

### Đã lược bỏ: module task nội bộ

27 request của `03 · Tổ`, `05 · Task`, `06 · Phiếu sửa` đã **xoá khỏi collection và khỏi
`scripts/gen_postman.py`**. Lý do: các route `/api/v1/tasks/*`, `/api/v1/teams/*`,
`/api/v1/revisions/*` chỉ được đăng ký khi `TASK_ENGINE_ENABLED=true` (`router.go`
`registerTaskRoutes`), còn khi tích hợp CMS thì đơn/task do CMS sở hữu — `.env` để `false`
nên chúng luôn trả `404 route_not_found`.

Code Go của module task **vẫn còn** (xem [07-task.md](07-task.md)); chỉ phần Postman bị bỏ.
Nếu bật lại `TASK_ENGINE_ENABLED=true` thì phải viết lại các request đó.

Các biến chỉ dùng cho module này cũng đã bỏ: `team_id`, `task_id`, `task_code`,
`assignment_id`, `revision_id`.

Mỗi request có mô tả (ràng buộc, mã lỗi có thể gặp) và test script kiểm tra status + trường quan trọng.

## 5. Chạy tự động (CI)

```bash
make postman-run TOKEN=$(make -s token) USER2=<uuid-nguoi-thu-2>
```

Kết quả lần chạy mới nhất trên stack dev: FLOW **17/17**, folder thường **50/50**,
folder CMS **37/37** — 0 fail.

`make postman-lint` kiểm file JSON: request trùng tên, thiếu method/URL, body không phải JSON
hợp lệ, và **file trên đĩa lệch với script sinh ra nó** (sửa tay trong app rồi quên cập nhật
script là lỗi hay gặp nhất).

## 6. WebSocket & gRPC trong app Postman

Định dạng v2.1 không chứa được 2 protocol này
([tài liệu Postman](https://learning.postman.com/docs/getting-started/importing-and-exporting/exporting-data)),
nên mẫu nằm trong **mô tả** của folder `10 · WebSocket & gRPC (mẫu frame)`: mở folder trong
Postman → tab **Documentation** → copy khối JSON cần dùng.

Trong đó có sẵn:

- **2 URL WebSocket** và khác nhau ở đâu (`/api/client/v1/chat/ws` cho FE, `/ws` cho công cụ nội bộ)
- **8 frame gửi lên** có body đầy đủ: `ping`, `message.send` (text / thread / kèm tệp),
  `message.read`, `typing`, `message.delete`, `template.send`
- **frame nhận về**: `ready`, `ack`, `message`, và bảng 7 event còn lại
- **bảng 13 trường hợp hay gặp**: thiếu token, token hết hạn, không phải thành viên,
  `clientMsgId` trùng, JSON sai, frame lạ, ping/pong timeout, mất kết nối…
- **5 message mẫu cho gRPC** + cách gắn metadata `authorization`

Giao thức đầy đủ: [02-websocket.md](02-websocket.md).

## 7. Khi API đổi

1. Sửa `scripts/gen_postman.py` (thêm/sửa request, mô tả, assertion).
2. `make postman` → sinh lại 2 file JSON.
3. `make postman-lint` và `make postman-run TOKEN=...` để chắc chắn còn chạy đúng.



---
