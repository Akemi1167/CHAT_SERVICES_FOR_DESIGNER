# 13 — Hướng dẫn test

Ba cách, từ nhanh đến đầy đủ. Mọi cách đều dùng chung dữ liệu do `make seed` dựng sẵn.

## 0. Khởi động + dựng dữ liệu (1 lần)

```bash
make dev-up      # Postgres + Redis + service (http://localhost:8080)
make seed        # 3 người dùng, 1 nhóm, tin nhắn + reply + thread + 1 yêu cầu @bot
```

`make seed` in ra: `conversation_id`, id tin gốc có thread, **token 7 ngày của 3 người** và **URL
WebSocket dán thẳng được**. Chạy lại bao nhiêu lần cũng được (tạo thêm nhóm mới, không phá dữ liệu cũ).

Trỏ sang máy khác: `make seed BASE=http://<vps>:8080`.

## 1. Nhanh nhất — chat trên terminal

Mở 2 cửa sổ, mỗi cửa sổ một người (lấy `CONV` từ output của `make seed`):

```bash
make ws SUB=demo-an   NAME=An   CONV=<conversation_id>
make ws SUB=demo-binh NAME=Binh CONV=<conversation_id>
```

Gõ chữ rồi Enter → cửa sổ kia hiện ngay. Lệnh trong client: `/ping`, `/typing`,
`/read <message_id>`, `/json {...}` (gửi frame bất kỳ), `/quit`.

## 2. Postman — bấm thử từng API

1. Import 2 file: `postman/chat-service.postman_collection.json` và
   `postman/chat-service.local.postman_environment.json`.
2. Chọn environment **Chat Service · Local**, dán token (từ `make seed`) vào biến `token`.
3. Chạy từ folder `00` xuống `07` — mỗi request tự lưu id cho request sau.

Muốn chạy cả bộ một lượt bằng CLI:

```bash
make postman-run TOKEN=$(make -s token SUB=demo-an NAME=An ROLE=order)
```

Kết quả mong đợi: **60 request / 91 assertion / 0 fail**.

**Test WebSocket trong Postman**: New → WebSocket → dán URL `ws://…/ws?token=…` (lấy nguyên dòng
`ws :` trong output `make seed`) → **Connect**. Mở 3 tab cho 3 người, **Connect hết rồi mới gửi**
(WebSocket không phát lại tin cũ). Frame gửi thử:

```json
{"id":"1","type":"message.send","payload":{"conversation_id":"<conv>","content":"Chào cả room"}}
```

> Đừng đặt `client_msg_id` cố định như `"m1"`: gửi lại giá trị cũ bị coi là retry → server trả
> `duplicated: true` và **không phát cho ai**. Bỏ trống là an toàn nhất khi test tay.

Tab nào nghi đã chết thì gửi `{"id":"x","type":"ping"}` — có `pong` là còn sống. Hoặc gọi
`GET /api/v1/ws/online` để xem ai đang kết nối.

## 3. curl — kiểm tra từng thứ

```bash
TOKEN=<token từ make seed>
CONV=<conversation_id>

curl -s localhost:8080/api/v1/me -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/conversations?limit=10" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/conversations/$CONV/messages?limit=20" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/messages/<message_id>/replies?mode=thread" -H "Authorization: Bearer $TOKEN" | jq
curl -s "localhost:8080/api/v1/messages/search?q=banner&conversation_id=$CONV" -H "Authorization: Bearer $TOKEN" | jq
curl -s -X POST "localhost:8080/api/v1/conversations/$CONV/read" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"message_id":"<message_id>"}' | jq
```

API cho FE (envelope `{statusCode, code, message, element}`): đổi tiền tố thành
`/api/client/v1/chat/...`, ví dụ `GET /api/client/v1/chat/bootstrap`.

## 4. Test code (không cần service chạy)

```bash
make test            # unit test toàn bộ
make lint            # golangci-lint (cần cài: make setup)
make dev-test        # chạy test bên trong container dev
```

## 5. Khi có lỗi — nhìn đâu

| Hiện tượng | Lệnh / chỗ xem |
|---|---|
| Request lỗi 4xx/5xx | `make dev-logs` — log JSON có `request_id`, `path`, `status` |
| Gửi tin mà người khác không nhận | `make dev-logs \| grep "deliver event"` → `delivered_connections` và `not_connected_here` |
| Không chắc tab WebSocket còn sống | `GET /api/v1/ws/online` hoặc gửi frame `ping` |
| Tin nhắn "biến mất" | tìm `bo qua tin nhan trung client_msg_id` trong log (trùng `client_msg_id`) |
| Nghi sai dữ liệu | `make dev-psql` rồi `select id, content, reply_to_id, thread_id from messages order by created_at desc limit 10;` |

## 6. Dọn dẹp

```bash
make dev-down        # dừng, giữ dữ liệu
make dev-down-v      # dừng + xoá sạch dữ liệu (chạy make seed lại để có bộ mới)
```
