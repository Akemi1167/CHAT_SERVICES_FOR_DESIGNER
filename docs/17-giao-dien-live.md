# 17 — Giao diện live (`web/live.html`)

Bản chạy thật của `chat-thread-demo.html`: **giữ nguyên toàn bộ CSS và khung HTML của demo**,
nhưng mọi dữ liệu lấy từ API thật thay vì IndexedDB.

```
http://localhost:8080/demo/live.html
```

## Mở lên

```bash
make up                                   # Chat + Postgres + Redis
TOK=$(make token-cms CMS_USER=6 | tail -1)   # token dev đóng vai user CMS
echo "$TOK"                               # dán vào ô token trên trang
open http://localhost:8080/demo/live.html
```

Trang phục vụ **cùng origin** với API nên không vướng CORS và WebSocket nối thẳng được.
Token lưu trong `localStorage` (bỏ tick "Ghi nhớ" nếu không muốn). Ở production tắt bằng
`WEB_DEMO_DIR=""` — `docker-compose.prod.yml` đã đặt sẵn.

## Đã nối những gì

| Thao tác trên giao diện | API gọi |
|---|---|
| Mở trang | `GET /chat/bootstrap` → danh sách nhóm, hồ sơ, thông báo |
| (ngay sau đó) | `WS /ws?token=…` → realtime |
| Bấm một nhóm | `GET /chat/groups/{id}/messages?limit=40` |
| Cuộn lên đầu | `GET …/messages?cursor=…` (tự động) |
| Nhìn thấy tin mới | `POST /chat/groups/{id}/read` |
| Gõ Enter | WS `message.send` (WS đứt → `POST …/messages`) |
| Đang gõ | WS `typing` → người khác thấy "… đang nhập" ở tiêu đề |
| Bấm "N phản hồi" | `GET /chat/messages/{id}/replies?mode=thread` |
| Trả lời trong thread | WS `message.send` kèm `thread_id` |
| Bấm nút trên card | `POST /chat/messages/{id}/actions` |
| "Tạo yêu cầu" | `POST /chat/forms/create_task/submit` |
| Nút bút chì (sidebar) | `POST /chat/groups/refresh` — hỏi lại CMS ngay |
| Nút thông tin (header) | thành viên + thông báo của nhóm |

Realtime xử lý đủ: `message`, `message.deleted`, `read`, `typing`, `presence`, `notification`,
`reload` (tự `bootstrap` lại). Mất kết nối thì tự nối lại với backoff, và nhãn ở góc trái dưới
chuyển sang **mất kết nối**.

## Hai điểm kỹ thuật đáng nhớ

**REST trả camelCase, WebSocket trả snake_case.** `GET /messages` cho `{text, senderId, createdAt}`
còn event realtime đẩy thẳng `domain.Message` nên là `{content, sender_id, created_at}`.
`normalize()` trong `web/live.js` gom hai dạng về một. FE thật cũng phải làm vậy.

**Card do CMS dựng, Chat không hiểu nội dung.** Bộ render card (`cardElementHTML`) đi theo kiểu
"biết gì vẽ nấy": `text`, `note`, `divider`, `field/fields`, `button`, `actions/row/group/section`,
`list`. Gặp `type` lạ thì vẫn in chữ ra chứ không bỏ im lặng. Khi CMS gửi tài liệu
`CHAT_SERVICE_UI_API.md` mục 2 (từ vựng card đầy đủ) thì bổ sung thêm case vào đúng hàm đó.

## Màn hình của demo bị ẩn

"Đơn hàng", "Tệp", "Work item", "Phiếu sửa", "Tổ" — **CMS sở hữu dữ liệu này**, Chat không có API
cho chúng. Khung HTML vẫn nằm trong trang (đã ẩn bằng `#live-overrides`), nên khi CMS mở API thì
bật lại chỉ là chuyện nối dữ liệu, không phải dựng lại giao diện.

## Đã kiểm chứng trên trình duyệt

Mở Chrome với nhóm thật **TEAM TEST** của CMS:

- đăng nhập bằng token → sidebar 1 nhóm, tiêu đề `4 thành viên · 1 đang online · DEV-TEST`;
- 15 tin nhắn lịch sử render đúng, có nhãn ngày "Hôm qua"/"Hôm nay";
- card thật `WEB-20261118-0006 · Đang làm · Banner Tết 2027` hiện kèm nút `submit_work`;
- gõ Enter → tin xuất hiện, WS ack;
- mở thread → bố cục chuyển sang 3 cột, panel phải hiện card gốc + phản hồi;
- mở tab thứ hai bằng token của `tester1`, gửi một tin → tab thứ nhất nhận **tức thì**, hiển thị
  là tin đến và preview ở sidebar đổi theo.
