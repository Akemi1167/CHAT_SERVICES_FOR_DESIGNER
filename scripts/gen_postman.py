#!/usr/bin/env python3
"""Sinh Postman collection (schema v2.1) + environment cho chat-service.

Chay lai moi khi doi API:  make postman
Nguon su that van la code/proto; file nay chi dich sang dang Postman import duoc.
"""
import json
import pathlib

OUT_DIR = pathlib.Path(__file__).resolve().parent.parent / "postman"
COLLECTION_FILE = OUT_DIR / "chat-service.postman_collection.json"
ENV_FILE = OUT_DIR / "chat-service.local.postman_environment.json"

SCHEMA = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
# ID on dinh de import nhieu lan khong tao ban trung.
COLLECTION_ID = "6f1a0c54-3c9f-4f0e-9a2a-1b7c5d0e1001"
ENV_ID = "6f1a0c54-3c9f-4f0e-9a2a-1b7c5d0e2001"


def js(*lines):
    """Script test/prerequest: tra ve dang Postman event."""
    return [{"listen": "test", "script": {"type": "text/javascript", "exec": list(lines)}}]


OK = 'pm.test("200/201 OK", () => pm.expect(pm.response.code).to.be.oneOf([200, 201]));'


def save(var, path, label=None):
    """Snippet luu 1 gia tri tu response vao collection variable."""
    return (
        f'const _d = pm.response.json().data;',
        f'if (_d) {{ pm.collectionVariables.set("{var}", {path}); '
        f'console.log("{label or var} =", pm.collectionVariables.get("{var}")); }}',
    )


def req(name, method, path, *, body=None, query=None, desc="", tests=None, auth=None, headers=None):
    url = {"raw": "{{base_url}}" + path, "host": ["{{base_url}}"]}
    clean = path.split("?")[0].strip("/")
    url["path"] = clean.split("/") if clean else []
    if query:
        url["query"] = [{"key": k, "value": v} for k, v in query]
        qs = "&".join(f"{k}={v}" for k, v in query)
        url["raw"] = "{{base_url}}" + clean_path(path) + "?" + qs
    item = {
        "name": name,
        "request": {
            "method": method,
            "header": ([{"key": "Content-Type", "value": "application/json"}] if body else [])
                      + [{"key": k, "value": v} for k, v in (headers or {}).items()],
            "url": url,
            "description": desc,
        },
        "response": [],
    }
    if body is not None:
        item["request"]["body"] = {
            "mode": "raw",
            "raw": json.dumps(body, ensure_ascii=False, indent=2),
            "options": {"raw": {"language": "json"}},
        }
    if auth is not None:
        item["request"]["auth"] = auth
    item["event"] = js(*(tests or (OK,)))
    return item


def clean_path(path):
    return path.split("?")[0]


def folder(name, desc, items):
    return {"name": name, "description": desc, "item": items}


# ---------------------------------------------------------------- folders

start = folder(
    "00 · Bắt đầu",
    "Kiểm tra service sống và lấy `user_id` từ token. **Chạy `Me` trước tiên** — các request sau "
    "dùng `{{user_id}}`.\n\nLấy token dev: `make token SUB=user-001 NAME=\"Nguyen Van A\" ROLE=designer` "
    "rồi dán vào biến `token` của environment.",
    [
        req("Healthz (liveness)", "GET", "/healthz", desc="Không cần token.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("status=ok", () => pm.expect(pm.response.json().data.status).to.eql("ok"));')),
        req("Readyz (ping DB)", "GET", "/readyz", desc="Không cần token. 503 nếu mất DB.",
            auth={"type": "noauth"}),
        req("Ai đang mở WebSocket (debug realtime)", "GET", "/api/v1/ws/online",
            desc="Trả về các kết nối WebSocket đang mở **trên instance này**. Dùng khi test: mở "
                 "các tab WS xong thì bấm request này để chắc chắn tab của người nhận còn sống "
                 "trước khi gửi tin. Chạy nhiều instance thì mỗi instance chỉ thấy kết nối của nó.",
            tests=(OK,
                   'const d = pm.response.json().data;',
                   'console.log("Dang ket noi:", d.instance_total_connections,',
                   '  "| users:", (d.online_users || []).map(u => (u.display_name || u.user_id) + "(" + u.connections + ")").join(", "));',
                   'pm.test("co truong instance_total_connections",'
                   ' () => pm.expect(d).to.have.property("instance_total_connections"));')),
        req("Me (token → user)", "GET", "/api/v1/me",
            desc="Xác thực token và upsert user. Lưu `user_id` cho các request sau.",
            tests=(OK, *save("user_id", "_d.user_id"),
                   'pm.test("co external_id", () => pm.expect(pm.response.json().data.external_id).to.be.a("string"));')),
    ],
)

conversations = folder(
    "01 · Hội thoại",
    "Chat nhóm và chat riêng. `Tạo nhóm` lưu `conversation_id` — mọi folder sau đều dùng biến này.",
    [
        req("Tạo nhóm", "POST", "/api/v1/conversations",
            body={"type": "group", "name": "Tổ Design · Đơn hàng 11",
                  "member_user_ids": []},
            desc="`type=group` bắt buộc có `name`. Người tạo là `owner`, người được thêm là `member`.",
            tests=(OK, *save("conversation_id", "_d.id"),
                   'pm.test("la group", () => pm.expect(pm.response.json().data.type).to.eql("group"));')),
        req("Tạo chat riêng (1-1)", "POST", "/api/v1/conversations",
            body={"type": "direct", "member_user_ids": ["{{user_id_2}}"]},
            desc="Cần `user_id_2` (gọi `/me` bằng token người thứ 2). Gọi 2 lần trả về **cùng** "
                 "một hội thoại (chống tạo trùng). Nếu `user_id_2` = chính mình → lỗi `invalid_direct_members`.",
            tests=('pm.test("200 hoặc 400 nếu chưa có user thứ 2",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 400]));',
                   'if (pm.response.code < 300) pm.collectionVariables.set("direct_conversation_id", pm.response.json().data.id);')),
        req("Danh sách hội thoại", "GET", "/api/v1/conversations", query=[("limit", "20")],
            desc="Mới nhất trước, kèm `unread_count`. Trang sau: thêm `cursor` = `meta.next_cursor`.",
            tests=(OK, 'pm.test("co meta", () => pm.expect(pm.response.json()).to.have.property("meta"));')),
        req("Chi tiết hội thoại", "GET", "/api/v1/conversations/{{conversation_id}}",
            desc="Kèm danh sách thành viên và role (owner/admin/member)."),
        req("Thêm thành viên (join room)", "POST", "/api/v1/conversations/{{conversation_id}}/members",
            body={"user_ids": ["{{user_id_2}}"], "role": "member"},
            desc="Đây chính là \"join room\": tư cách thành viên lưu trong DB, không phải frame "
                 "WebSocket. Chỉ owner/admin. Thêm lại người đã có → idempotent. "
                 "Người được thêm nếu đang mở WebSocket sẽ nhận `conversation.updated` và từ đó "
                 "nhận mọi `message.new` của nhóm.",
            tests=('const u2 = pm.variables.get("user_id_2") || "";',
                   'if (u2.length === 36) {',
                   '  pm.test("200 OK (them thanh vien thanh cong)", () => pm.response.to.have.status(200));',
                   '  pm.test("user co trong danh sach thanh vien", () => {',
                   '    const ids = (pm.response.json().data.members || []).map(m => m.user_id);',
                   '    pm.expect(ids).to.include(u2);',
                   '  });',
                   '} else {',
                   '  pm.test("400 vi chua dien user_id_2", () => pm.response.to.have.status(400));',
                   '}')),
    ],
)

messages = folder(
    "02 · Tin nhắn",
    "Gửi/đọc tin nhắn, thread theo nhiệm vụ, trạng thái đã xem, tìm kiếm.",
    [
        req("Gửi tin nhắn text", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"type": "text", "content": "Chào cả nhà 👋", "client_msg_id": "{{$guid}}"},
            desc="`client_msg_id` do FE sinh và phải **duy nhất mỗi tin**: gửi lại đúng id cũ sẽ "
                 "trả về tin cũ với `duplicated=true`, **không tạo tin mới và không broadcast**. "
                 "Ở đây dùng `{{$guid}}` nên mỗi lần Send là một id mới.",
            tests=(OK, *save("message_id", "_d.message.id"),
                   'pm.collectionVariables.set("thread_id", pm.response.json().data.message.id);')),
        req("Gửi kèm ảnh/file", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"type": "image", "content": "Mẫu banner v1",
                  "attachments": [{"kind": "image", "url": "https://picsum.photos/800/600",
                                   "name": "banner-v1.png", "mime_type": "image/png",
                                   "size_bytes": 102400, "width": 800, "height": 600}],
                  "client_msg_id": "{{$guid}}"},
            desc="Service chỉ lưu URL — việc upload do service file/CDN khác làm."),
        req("Trả lời trong thread", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"content": "Phản hồi trong thread của nhiệm vụ", "thread_id": "{{thread_id}}",
                  "client_msg_id": "{{$guid}}"},
            desc="`thread_id` = id tin nhắn gốc (hoặc card task). Phản hồi hiện cả trong nhóm và trong thread."),
        req("Trả lời (trích dẫn) một tin nhắn", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"content": "Mình nhận nhé", "reply_to_id": "{{message_id}}",
                  "thread_id": "{{thread_id}}"},
            desc="Hai quan hệ khác nhau:\n\n"
                 "- `reply_to_id` = tin được **trích dẫn** (nút Reply trên UI)\n"
                 "- `thread_id` = tin **mở thread** (card yêu cầu) — phản hồi hiện cả trong nhóm và trong thread\n\n"
                 "Cả hai phải thuộc cùng hội thoại, nếu không → 400 `invalid_reply_to_id` / `invalid_thread_id`.",
            tests=(OK,
                   'const m = pm.response.json().data.message;',
                   'pm.collectionVariables.set("reply_message_id", m.id);',
                   'pm.test("co reply_to_id", () => pm.expect(m.reply_to_id).to.be.a("string"));')),
        req("Phản hồi của một tin nhắn", "GET", "/api/v1/messages/{{message_id}}/replies",
            query=[("mode", "direct"), ("limit", "30")],
            desc="`mode=direct` (mặc định): các tin **trả lời trực tiếp** tin này (`reply_to_id`).\n"
                 "`mode=thread`: **toàn bộ thread** tin này mở ra (`thread_id`) — dùng cho panel thread "
                 "của card/yêu cầu.\n\nTrả về `root` (tin gốc), `replies` (cũ → mới), `reply_count`, "
                 "`thread_count`; phân trang bằng `cursor` như các API list khác.",
            tests=(OK,
                   'const d = pm.response.json().data;',
                   'pm.test("co root + replies", () => { pm.expect(d).to.have.property("root"); pm.expect(d.replies).to.be.an("array"); });',
                   'pm.test("reply_count la so", () => pm.expect(d.reply_count).to.be.a("number"));')),
        req("Cả thread của một tin nhắn", "GET", "/api/v1/messages/{{message_id}}/replies",
            query=[("mode", "thread"), ("limit", "50")],
            desc="Giống panel thread trong demo: mọi tin trong thread, sắp xếp cũ → mới.",
            tests=(OK, 'pm.test("replies la mang", () => pm.expect(pm.response.json().data.replies).to.be.an("array"));')),
        req("Lịch sử tin nhắn", "GET", "/api/v1/conversations/{{conversation_id}}/messages",
            query=[("limit", "30")],
            desc="Mới nhất trước. Dùng `cursor` = `meta.next_cursor` để tải tiếp (đừng dùng offset).\n\n"
                 "Mỗi tin kèm `reply_count` (trả lời trực tiếp) và `thread_count` (số tin trong thread "
                 "tin đó mở) để UI hiện badge \"N phản hồi\" mà không phải gọi thêm API.",
            tests=(OK, 'pm.test("co mang data", () => pm.expect(pm.response.json().data).to.be.an("array"));')),
        req("Tin nhắn trong 1 thread", "GET", "/api/v1/conversations/{{conversation_id}}/messages",
            query=[("thread_id", "{{thread_id}}"), ("limit", "50")],
            desc="Trả cả tin nhắn gốc + mọi phản hồi trong thread."),
        req("Tìm tin nhắn", "GET", "/api/v1/messages/search",
            query=[("q", "banner"), ("conversation_id", "{{conversation_id}}"), ("limit", "20")],
            desc="Tìm theo nội dung, mã task (`metadata.task_code`), người gửi (`sender_id`), "
                 "khoảng ngày (`from_date`, `to_date` dạng YYYY-MM-DD)."),
        req("Đánh dấu đã đọc", "POST", "/api/v1/conversations/{{conversation_id}}/read",
            body={"message_id": "{{message_id}}"},
            desc="Trả về `unread_count` còn lại. Mốc đã đọc chỉ tiến, không lùi.",
            tests=(OK, 'pm.test("co unread_count", () => pm.expect(pm.response.json().data).to.have.property("unread_count"));')),
        req("Ai đã xem tin này", "GET", "/api/v1/messages/{{message_id}}/readers",
            desc="Danh sách người đã đọc (bảng `message_receipts`)."),
        req("Xoá tin nhắn (mềm)", "DELETE", "/api/v1/messages/{{message_id}}",
            desc="Người gửi hoặc owner/admin nhóm. ⚠️ Chạy cuối vì các request trên dùng `message_id`.",
            tests=('pm.test("200 hoặc 403", () => pm.expect(pm.response.code).to.be.oneOf([200, 403]));',)),
    ],
)

teams = folder(
    "03 · Tổ & gợi ý người phụ trách",
    "Tổ (bộ phận) dùng cho `chọn tổ` khi tạo yêu cầu. **Tổ phải có thành viên** thì bot/`/tasks` mới "
    "gợi ý được người phụ trách.",
    [
        req("Danh sách tổ", "GET", "/api/v1/teams",
            desc="Seed sẵn: `design`, `edit`, `order`. Lưu `team_id` của tổ design.",
            tests=(OK,
                   'const d = pm.response.json().data;',
                   'const design = d.find(t => t.code === "design") || d[0];',
                   'if (design) pm.collectionVariables.set("team_id", design.id);')),
        req("Thêm mình vào tổ design", "POST", "/api/v1/teams/{{team_id}}/members",
            body={"user_ids": ["{{user_id}}"], "role": "member"},
            desc="`role`: leader (tổ trưởng) | assistant (trợ lý) | member. Thêm lại → idempotent."),
        req("Thành viên của tổ", "GET", "/api/v1/teams/{{team_id}}/members"),
        req("Gợi ý người phụ trách", "GET", "/api/v1/teams/{{team_id}}/suggest",
            desc="Xếp hạng: người **không OFF** trước, rồi **ít việc đang mở nhất** (`open_tasks`), "
                 "rồi theo tên. FE hiển thị gợi ý nhưng vẫn cho đổi người."),
        req("Tạo tổ mới", "POST", "/api/v1/teams",
            body={"code": "design-2", "name": "Tổ Design 2", "kind": "design"},
            desc="`kind`: design | edit | order | other.",
            tests=('pm.test("201 hoặc 409 nếu đã tồn tại",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 409]));',)),
    ],
)

bots = folder(
    "04 · Bot @bot & gửi mẫu",
    "Gọi bot bằng `@bot <mã_mẫu> trường=giá_trị`. Bot luôn lưu mẫu vào `bot_submissions`, "
    "mẫu `yeu_cau`/`phieu_sua` còn tạo task/phiếu sửa thật.",
    [
        req("Danh sách bot", "GET", "/api/v1/bots"),
        req("Schema các mẫu (FE render form)", "GET", "/api/v1/bots/{{bot_slug}}/templates",
            desc="Trả `schema.fields` (key/label/type/required) để FE dựng form `gửi mẫu`.",
            tests=(OK, 'pm.test("co templates", () => pm.expect(pm.response.json().data.templates).to.be.an("array"));')),
        req("@bot (không có mẫu → help)", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"content": "@bot", "client_msg_id": "{{$guid}}"},
            desc="Bot trả về danh sách mẫu đang có.",
            tests=(OK, 'pm.test("bot tra loi", () => pm.expect(pm.response.json().data.bot_reply).to.not.eql(null));')),
        req("@bot yeu_cau → tạo task", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"content": '@bot yeu_cau tieu_de="Banner Tết 2027" to=design so_luong=3 '
                             'dinh_muc=A4 deadline=2026-12-01 ghi_chu="Làm gấp"',
                  "client_msg_id": "{{$guid}}"},
            desc="Cú pháp `key=value` (dùng `\"...\"` cho giá trị có khoảng trắng) hoặc JSON: "
                 "`@bot yeu_cau {\"tieu_de\":\"...\"}`.\n\nBot lưu mẫu (`submission.status=done`) rồi "
                 "tạo task — **card task chính là câu trả lời** nên `bot_reply=null`. "
                 "Thiếu trường bắt buộc → `bot_reply` ghi rõ thiếu gì và submission = `failed`.",
            tests=(OK,
                   'const d = pm.response.json().data;',
                   'pm.test("la bot_command", () => pm.expect(d.message.type).to.eql("bot_command"));',
                   'pm.test("mau da luu", () => pm.expect(d.submission.status).to.be.oneOf(["done", "stored"]));')),
        req("Gửi mẫu từ form (không gõ @bot)", "POST", "/api/v1/bots/{{bot_slug}}/submissions",
            body={"conversation_id": "{{conversation_id}}", "template_code": "phieu_sua",
                  "payload": {"ma_task": "{{task_code}}", "nguyen_nhan": "Sai màu nền",
                              "loai_sua": "màu", "so_luong": 2},
                  "client_msg_id": "{{$guid}}"},
            desc="Dùng khi FE có form riêng. ⚠️ Cần `task_code` (chạy folder 05 trước) vì mẫu "
                 "`phieu_sua` tạo phiếu sửa cho task đó.",
            tests=('pm.test("201 hoặc 400 nếu chưa có task_code",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 400]));',)),
        req("Các mẫu đã lưu", "GET", "/api/v1/bots/{{bot_slug}}/submissions",
            query=[("conversation_id", "{{conversation_id}}"), ("limit", "20")],
            desc="Lọc thêm `status` = stored | processing | done | failed."),
    ],
)

tasks = folder(
    "05 · Task (yêu cầu / nhận việc)",
    "Vòng đời card của từng người. Trạng thái task tổng được **suy ra** từ các card.\n\n"
    "Chạy theo thứ tự: Tạo → Danh sách (lưu `task_id`) → Nhận → Tạm dừng → Tiếp tục → Hoàn thành.",
    [
        req("Tạo yêu cầu (giao cho mình)", "POST", "/api/v1/tasks",
            body={"conversation_id": "{{conversation_id}}", "title": "Video quảng cáo Tết",
                  "description": "Dựng 15s theo kịch bản", "team_code": "design",
                  "quantity": 10, "quota": "HD", "priority": "high",
                  "deadline": "2026-12-20", "assignee_ids": ["{{user_id}}"]},
            desc="Bỏ trống `assignee_ids` → hệ thống tự gợi ý người trong tổ. "
                 "Người được giao mà chưa ở trong nhóm sẽ được tự thêm vào nhóm.",
            tests=(OK, *save("task_id", "_d.id"),
                   'pm.collectionVariables.set("task_code", pm.response.json().data.code);',
                   'const a = pm.response.json().data.assignments[0];',
                   'if (a) pm.collectionVariables.set("assignment_id", a.id);',
                   'pm.test("cho nhan", () => pm.expect(pm.response.json().data.status).to.eql("waiting_accept"));')),
        req("Danh sách task", "GET", "/api/v1/tasks",
            query=[("conversation_id", "{{conversation_id}}"), ("limit", "20")],
            desc="Lọc: `team_id`, `assignee_id`, `created_by`, `status` (nhiều giá trị cách nhau bởi `,`), "
                 "`thread_id`, `q`, `mine=true`, `waiting_accept=true`."),
        req("Chờ nhận (của tôi)", "GET", "/api/v1/tasks",
            query=[("waiting_accept", "true"), ("mine", "true"), ("limit", "20")],
            desc="Task đã giao nhưng chưa nhận — màn hình `Chờ nhận` của designer."),
        req("Chi tiết task", "GET", "/api/v1/tasks/{{task_id}}",
            desc="Kèm card của từng người, phiếu sửa, tổ."),
        req("Tra cứu theo mã task", "GET", "/api/v1/tasks/code/{{task_code}}",
            desc="Mã dạng `T-000001` — tiện khi người dùng nhắc mã trong chat."),
        req("Nhận task (định mức + số lượng)", "POST", "/api/v1/tasks/{{task_id}}/accept",
            body={"quota": "HD-cao-cấp", "quantity": 5},
            desc="**Bắt buộc** chọn định mức và số lượng trước khi nhận; số lượng không vượt "
                 "`quantity` của task. Nhận lại lần 2 → 409 `task_already_accepted`.",
            tests=(OK, 'pm.test("dang lam", () => pm.expect(pm.response.json().data.status).to.eql("in_progress"));')),
        req("Tạm dừng", "POST", "/api/v1/tasks/{{task_id}}/pause",
            body={"note": "Chờ file gốc từ order"},
            tests=(OK, 'pm.test("tam dung", () => pm.expect(pm.response.json().data.status).to.eql("paused"));')),
        req("Tiếp tục", "POST", "/api/v1/tasks/{{task_id}}/resume", body={"note": ""},
            tests=(OK, 'pm.test("dang lam", () => pm.expect(pm.response.json().data.status).to.eql("in_progress"));')),
        req("Treo (bắt buộc ghi lý do)", "POST", "/api/v1/tasks/{{task_id}}/hold",
            body={"note": "Chờ duyệt mẫu từ khách"},
            desc="Thiếu `note` → 400 `hold_requires_note`.",
            tests=(OK, 'pm.test("treo", () => pm.expect(pm.response.json().data.status).to.eql("on_hold"));')),
        req("Tiếp tục sau khi treo", "POST", "/api/v1/tasks/{{task_id}}/resume", body={"note": ""}),
        req("Hoàn thành", "POST", "/api/v1/tasks/{{task_id}}/complete", body={"note": "Đã nộp file"},
            desc="Bị chặn (409 `task_has_open_revision`) nếu còn phiếu sửa đang mở. "
                 "Chuyển trạng thái sai (vd đang `paused` mà hoàn thành) → 409 `invalid_task_transition` "
                 "kèm `details.allowed`.",
            tests=(OK, 'pm.test("hoan thanh", () => pm.expect(pm.response.json().data.status).to.eql("completed"));')),
        req("Chia task / gọi hỗ trợ / gọi Edit", "POST", "/api/v1/tasks/{{task_id}}/assignees",
            body={"user_ids": ["{{user_id_2}}"], "role": "support", "note": "Hỗ trợ phần hình"},
            desc="`role`: primary (chia việc, mỗi người 1 card) | support (gọi hỗ trợ) | edit (gọi Edit làm video). "
                 "Chỉ owner/admin nhóm hoặc người tạo task.",
            tests=('pm.test("200 hoặc 400/409 nếu chưa có user thứ 2",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 409]));',)),
        req("Đổi người phụ trách", "POST", "/api/v1/tasks/{{task_id}}/reassign",
            body={"to_user_id": "{{user_id_2}}", "note": "Người cũ đang quá tải"},
            desc="Card cũ → `revoked`, người mới nhận card `pending`. 409 `same_assignee` nếu "
                 "`user_id_2` là chính mình, 409 `already_assigned` nếu người đó đã có card trên "
                 "task (ví dụ vừa được chia việc ở request trên) — muốn thấy 200 thì dùng người "
                 "thứ 3 chưa có card.",
            tests=('pm.test("200 hoặc 400/409 nếu chưa có user thứ 2",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 409]));',)),
        req("Điều chỉnh thông tin / đổi tổ", "PATCH", "/api/v1/tasks/{{task_id}}",
            body={"title": "Video quảng cáo Tết - bản 2", "team_code": "edit",
                  "quantity": 12, "quota": "4K", "deadline": "2026-12-25", "priority": "urgent"},
            desc="Chỉ gửi field muốn đổi. `clear_deadline=true` để bỏ deadline. "
                 "Designer thường sửa → 403 `require_task_manager`."),
        req("Lịch sử task", "GET", "/api/v1/tasks/{{task_id}}/events", query=[("limit", "50")],
            desc="Ai làm gì, lúc nào, từ trạng thái nào sang trạng thái nào."),
    ],
)

revisions = folder(
    "06 · Phiếu sửa",
    "Ghi nguyên nhân / loại sửa / số lượng. Nếu card đã hoàn thành thì **tự mở lại** để sửa, "
    "và mỗi phiếu được hoàn thành **riêng**.",
    [
        req("Tạo phiếu sửa", "POST", "/api/v1/tasks/{{task_id}}/revisions",
            body={"reason": "Sai màu nền so với brief", "revision_type": "màu",
                  "quantity": 2, "assignment_id": "{{assignment_id}}"},
            desc="Bắt buộc `reason` + `revision_type`. Bỏ `assignment_id` → tính cho card phụ trách chính.",
            tests=(OK,
                   'const d = pm.response.json().data;',
                   'pm.collectionVariables.set("revision_id", d.revision.id);',
                   'pm.test("phieu dang mo", () => pm.expect(d.revision.status).to.eql("open"));',
                   'pm.test("card mo lai", () => pm.expect(d.task.status).to.be.oneOf(["in_progress", "waiting_accept"]));')),
        req("Danh sách phiếu sửa của task", "GET", "/api/v1/tasks/{{task_id}}/revisions"),
        req("Hoàn thành 1 phiếu sửa", "POST", "/api/v1/revisions/{{revision_id}}/complete",
            body={"note": "Đã sửa lại màu nền"},
            desc="Người phụ trách card hoặc admin. Tin nhắn hệ thống ghi rõ còn bao nhiêu phiếu đang mở.",
            tests=(OK, 'pm.test("da hoan thanh", () => pm.expect(pm.response.json().data.revision.status).to.eql("completed"));')),
        req("Hoàn thành card sau khi hết phiếu", "POST", "/api/v1/tasks/{{task_id}}/complete",
            body={"note": "Nộp lại file đã sửa"},
            desc="Trạng thái **task** được suy ra từ tất cả card: nếu task đã được chia cho người "
                 "khác mà họ chưa xong thì task vẫn `in_progress` dù card của bạn đã `completed`.",
            tests=(OK,
                   'const t = pm.response.json().data;',
                   'const mine = (t.assignments || []).find(a => a.user_id === pm.collectionVariables.get("user_id"));',
                   'pm.test("card cua toi hoan thanh", () => pm.expect(mine && mine.status).to.eql("completed"));',
                   'pm.test("task completed, hoac in_progress neu con card nguoi khac",'
                   ' () => pm.expect(t.status).to.be.oneOf(["completed", "in_progress"]));')),
    ],
)

errors = folder(
    "07 · Lỗi & quyền (xem cách API phản hồi)",
    "Mọi lỗi trả về `{\"error\":{\"code\",\"message\",\"details\"}}` với `code` ổn định để FE bắt.",
    [
        req("Không có token → 401", "GET", "/api/v1/conversations", auth={"type": "noauth"},
            desc="Code: `unauthenticated`.",
            tests=('pm.test("401", () => pm.response.to.have.status(401));',
                   'pm.test("code unauthenticated", () => pm.expect(pm.response.json().error.code).to.eql("unauthenticated"));')),
        req("UUID sai → 400", "GET", "/api/v1/tasks/khong-phai-uuid",
            tests=('pm.test("400", () => pm.response.to.have.status(400));',
                   'pm.test("code invalid_uuid", () => pm.expect(pm.response.json().error.code).to.eql("invalid_uuid"));')),
        req("Hội thoại không tồn tại → 403/404", "GET",
            "/api/v1/conversations/00000000-0000-0000-0000-000000000000/messages",
            desc="Không phải thành viên → `not_conversation_member` (không tiết lộ hội thoại có tồn tại hay không).",
            tests=('pm.test("403 hoặc 404", () => pm.expect(pm.response.code).to.be.oneOf([403, 404]));',)),
        req("Endpoint không tồn tại → 404", "GET", "/api/v1/khong-co-endpoint-nay",
            desc="Code: `route_not_found`. Lỗi của Fiber (404/405/413/upgrade) được map sang đúng "
                 "status thay vì 500.",
            tests=('pm.test("404", () => pm.response.to.have.status(404));',
                   'pm.test("code route_not_found", () => pm.expect(pm.response.json().error.code).to.eql("route_not_found"));')),
        req("Tin nhắn rỗng → 400", "POST", "/api/v1/conversations/{{conversation_id}}/messages",
            body={"content": "   "},
            tests=('pm.test("400", () => pm.response.to.have.status(400));',
                   'pm.test("code empty_message", () => pm.expect(pm.response.json().error.code).to.eql("empty_message"));')),
        req("Nhận task thiếu số lượng → 400", "POST", "/api/v1/tasks/{{task_id}}/accept",
            body={"quota": "HD"},
            tests=('pm.test("400 hoặc 409", () => pm.expect(pm.response.code).to.be.oneOf([400, 409]));',)),
    ],
)

cms_bot = folder(
    "08 · CMS → Chat (Bot API)",
    "Các API **CMS gọi sang Chat**. Auth: header `X-Chat-Service-Key` (biến `service_key`), "
    "không dùng JWT. Mọi request ghi nên có `Idempotency-Key` — gặp lại key trả nguyên kết quả cũ.\n\n"
    "Chỉ bật khi `CMS_ENABLED=true`. Chi tiết: `docs/10-cms-integration.md`.",
    [
        req("Resolve user (openId → actorId)", "POST", "/bot/v1/users/resolve",
            body={"items": [{"openId": "local:designer-1", "cmsUserId": 68, "name": "Trần Designer"}]},
            desc="Chưa có trong Chat thì tạo actor mới (lấy hồ sơ qua Workspace API của CMS).",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co actorId", () => pm.expect(pm.response.json().items[0]).to.have.property("actorId"));',
                   'const it = pm.response.json().items[0];',
                   'if (it) pm.collectionVariables.set("actor_id", it.actorId);')),
        req("Gửi card vào nhóm", "POST", "/bot/v1/messages",
            body={"groupId": "{{chat_id}}", "bot": {"id": 1, "name": "Demo Designer Bot"},
                  "card": {"schema": "abc.card/v1",
                           "header": {"code": "WEB-20261118-0006", "kind": "DESIGN",
                                      "title": "Banner Tết 2027",
                                      "status": {"text": "Chưa nhận", "tone": "warning"}},
                           "visibleTo": None,
                           "elements": [{"type": "button", "text": "Nhận task",
                                         "action": "accept_task", "visibleTo": ["{{actor_id}}"]}]}},
            desc="`groupId` nhận cả `wg_…` của CMS lẫn UUID nội bộ. Trả `{messageId, cardVersion}`.",
            auth={"type": "noauth"},
            tests=(OK, 'const d = pm.response.json();',
                   'pm.collectionVariables.set("card_message_id", d.messageId);',
                   'pm.test("cardVersion = 1", () => pm.expect(d.cardVersion).to.eql(1));')),
        req("Sửa card (PATCH theo cardVersion)", "PATCH", "/bot/v1/messages/{{card_message_id}}",
            body={"card": {"schema": "abc.card/v1",
                           "header": {"code": "WEB-20261118-0006", "title": "Banner Tết 2027",
                                      "status": {"text": "Đang làm", "tone": "info"}},
                           "elements": [{"type": "button", "text": "Nộp bài", "action": "submit_work"}]},
                  "cardVersion": 2},
            desc="Chỉ áp dụng khi `cardVersion` **lớn hơn** bản đang lưu; nhỏ hơn/bằng → `{\"ok\":true,\"ignored\":true}`.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("ok", () => pm.expect(pm.response.json().ok).to.be.true);')),
        req("Trả lời trong thread", "POST", "/bot/v1/messages/{{card_message_id}}/thread",
            body={"text": "Đã giao Trần Designer · Chờ nhận task.", "bot": {"id": 1, "name": "Demo Designer Bot"}},
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co messageId", () => pm.expect(pm.response.json()).to.have.property("messageId"));')),
        req("Thông báo cho 1 người", "POST", "/bot/v1/notifications",
            body={"actorId": "{{actor_id}}", "text": "Bạn có task mới: WEB-20261118-0006",
                  "messageId": "{{card_message_id}}"},
            auth={"type": "noauth"},
            tests=(OK,)),
        req("Đẩy nhóm + thành viên", "PUT", "/bot/v1/groups/{{chat_id}}",
            body={"name": "Nhóm Thiết kế Web",
                  "members": [{"openId": "local:designer-1", "cmsUserId": 68, "name": "Trần Designer",
                               "sources": ["team"], "teamCodes": ["DESIGN"]}]},
            desc="**Thay thế toàn bộ** danh sách thành viên: ai không còn trong danh sách mất quyền xem nhóm ngay.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co memberCount", () => pm.expect(pm.response.json()).to.have.property("memberCount"));')),
        req("Sai service key → 401", "POST", "/bot/v1/users/resolve",
            body={"items": []},
            headers={"X-Chat-Service-Key": "khoa-sai-co-y"},
            desc="Gửi khoá sai để xem dạng lỗi `{code, message, retryable}`.",
            auth={"type": "noauth"},
            tests=('pm.test("401", () => pm.response.to.have.status(401));',
                   'pm.test("code invalid_service_key",'
                   ' () => pm.expect(pm.response.json().code).to.eql("invalid_service_key"));')),
    ],
)

cms_client = folder(
    "09 · API cho FE (hợp đồng CMS)",
    "Bộ API `/api/client/v1/chat/*` với envelope `{statusCode, code, message, element}`.\n\n"
    "Auth: **access token do CMS phát** — đặt vào biến `cms_token` (lấy từ "
    "`POST /api/client/v1/workspace/auth/login` của CMS). Token dev `make token` cũng dùng được "
    "khi `AUTH_ALLOW_DEV_TOKEN=true`, nhưng sẽ không có `cmsUserId`/nhóm CMS.",
    [
        req("Bootstrap", "GET", "/api/client/v1/chat/bootstrap",
            desc="Tất cả thứ FE cần khi mở app: `me`, `groups` (chỉ nhóm mình là thành viên, "
                 "kèm `members` + `sources`/`teamCodes`), `notifications`, `unreadNotifications`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK, 'const el = pm.response.json().element;',
                   'pm.test("co me + groups", () => pm.expect(el).to.have.keys(["me","groups","notifications","unreadNotifications"]));',
                   'if (el.groups && el.groups[0]) pm.collectionVariables.set("chat_id", el.groups[0].chatId || el.groups[0].id);')),
        req("Tôi là ai", "GET", "/api/client/v1/chat/auth/me",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Đồng bộ lại nhóm từ CMS", "POST", "/api/client/v1/chat/groups/refresh",
            desc="Gọi sau khi user vào nhóm bằng link mời.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 hoac 400 neu chua bat CMS",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400]));',)),
        req("Lịch sử tin nhắn (card đã lọc)", "GET", "/api/client/v1/chat/groups/{{chat_id}}/messages",
            query=[("limit", "30")],
            desc="Card trả về đã **lọc nút theo `visibleTo`** của chính người gọi; `actions` liệt kê "
                 "action người đó được bấm.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Gửi tin nhắn", "POST", "/api/client/v1/chat/groups/{{chat_id}}/messages",
            body={"text": "Nhờ team làm gấp giúp mình nhé", "clientMsgId": "{{$guid}}"},
            desc="Chat lưu + phát realtime + gửi `message.created` sang CMS (bất đồng bộ qua outbox).",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Phản hồi của 1 tin nhắn", "GET", "/api/client/v1/chat/messages/{{card_message_id}}/replies",
            query=[("mode", "thread"), ("limit", "50")],
            desc="Trả `root`, `replies` (card đã lọc `visibleTo`), `replyCount`, `threadCount`. "
                 "`mode=direct` cho trích dẫn, `mode=thread` cho panel thread của card.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Bấm nút trên card", "POST", "/api/client/v1/chat/messages/{{card_message_id}}/actions",
            body={"action": "accept_task", "values": {"normItemId": "996802", "quantity": 2}},
            desc="Chat kiểm `visibleTo` trước (không được phép → `403 ACTION_NOT_ALLOWED`), rồi gửi "
                 "`card.action` sang CMS và áp card mới nếu `cardVersion` lớn hơn.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 hoac 403", () => pm.expect(pm.response.code).to.be.oneOf([200, 403]));',)),
        req("Gửi form tạo yêu cầu", "POST", "/api/client/v1/chat/forms/create_task/submit",
            body={"values": {"kind": "task", "title": "Banner Tết 2027", "teamId": "27", "useAI": False},
                  "context": {"groupId": "{{chat_id}}"}},
            desc="FE gửi `teamId` trong `values`; Chat **không diễn giải**, chuyển nguyên sang CMS "
                 "trong event `form.submit` rồi trả `toast`/`closeForm` cho UI.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 hoac 400 neu chua bat CMS",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400]));',)),
        req("Thông báo của tôi", "GET", "/api/client/v1/chat/notifications",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Đánh dấu đã đọc thông báo", "POST", "/api/client/v1/chat/notifications/read",
            body={"all": True},
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
    ],
)

cleanup = folder(
    "99 · Dọn dẹp (chạy cuối, tuỳ ý)",
    "Các request làm thay đổi/rút quyền — tách riêng để không phá flow ở trên.",
    [
        req("Huỷ task", "POST", "/api/v1/tasks/{{task_id}}/cancel",
            body={"reason": "Khách đổi yêu cầu"},
            desc="Huỷ sẽ thu hồi các card chưa hoàn thành → mọi thao tác sau đó trên task trả "
                 "409 `task_cancelled`. Vì vậy request này nằm ở folder dọn dẹp, chạy sau cùng.",
            tests=(OK, 'pm.test("da huy", () => pm.expect(pm.response.json().data.status).to.eql("cancelled"));')),
        req("Rời nhóm / xoá thành viên", "DELETE",
            "/api/v1/conversations/{{conversation_id}}/members/{{user_id_2}}",
            desc="Tự rời nhóm được; xoá người khác cần owner/admin.",
            tests=('pm.test("200 hoặc 400/403", () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
        req("Bỏ thành viên khỏi tổ", "DELETE", "/api/v1/teams/{{team_id}}/members/{{user_id}}",
            tests=('pm.test("200", () => pm.response.to.have.status(200));',)),
    ],
)

DESCRIPTION = """\
# Chat Service — ABC Workspace

Chat (riêng/nhóm) + bot `@bot` gửi mẫu + task / nhận task / phiếu sửa.
Go · Fiber (REST + WebSocket) · gRPC · PostgreSQL · GORM.

## Chuẩn bị (2 bước)

1. Chạy service: `make dev-up` (Docker, hot reload) hoặc `make up-deps && make migrate-up && make run`.
2. Lấy token dev và dán vào biến `token` của environment **Chat Service · Local**:
   `make token SUB=user-001 NAME="Nguyen Van A" ROLE=designer`
   Muốn thử phân công 2 người: tạo token thứ 2 (`SUB=user-002`), gọi `/me` bằng token đó,
   rồi dán `user_id` nhận được vào biến `user_id_2`.

## Cách chạy

Chạy lần lượt từ folder `00` xuống `06` (hoặc Run collection): mỗi folder tự lưu id cần thiết
(`user_id`, `conversation_id`, `message_id`, `task_id`, `task_code`, `assignment_id`, `revision_id`)
vào collection variable cho folder sau dùng.

Response thành công: `{"data": ..., "meta": {"next_cursor","has_more"}}`.
Lỗi: `{"error":{"code","message","details"}}` — `code` ổn định, dùng để FE bắt lỗi.
Phân trang: cursor (`?cursor=<meta.next_cursor>&limit=30`), **không** dùng offset.

## Tích hợp CMS

Folder `08 · CMS → Chat (Bot API)` và `09 · API cho FE (hợp đồng CMS)` chỉ chạy khi service bật
`CMS_ENABLED=true`. Điền 2 biến environment: `service_key` (= `CHAT_SERVICE_KEY`) và `cms_token`
(access token do CMS phát). Header `X-Chat-Service-Key` + `Idempotency-Key` được gắn tự động cho
mọi request `/bot/v1`. Chi tiết: `docs/10-cms-integration.md`.

## WebSocket & gRPC

Postman **không** cho export 2 loại request này trong file v2.1 (giới hạn của Postman),
nên chúng không có trong file import này. Hai cách dùng:

- **WebSocket**: New → WebSocket → `ws://localhost:8080/ws?token=<JWT>` → Connect.
  Kiểm tra frame `ready` xem `display_name` có đúng người không (token trong URL luôn thắng
  header `Authorization` mà Postman có thể tự gắn).
  Gửi frame: `{"id":"1","type":"message.send","payload":{"conversation_id":"...","content":"hello"}}`.
  Lưu ý: nếu thêm `client_msg_id` thì giá trị phải **khác nhau mỗi lần gửi** — trùng giá trị cũ sẽ
  bị coi là retry (`ack` trả `"duplicated": true`) và **không broadcast cho ai cả**.
  Các frame khác: `template.send`, `message.read`, `message.delete`, `typing`, `ping`.
  Server đẩy xuống: `ready`, `ack`, `error`, `message.new`, `message.read`, `typing`,
  `presence.update`, `task.created`, `task.updated`, `bot.submission.stored`.
  Chi tiết: `docs/02-websocket.md`.
- **gRPC**: New → gRPC → `localhost:9090` → chọn service (server reflection đang bật) →
  thêm metadata `authorization: Bearer <JWT>`. Có `ChatService`, `BotService`, `TaskService`.
  Chi tiết: `docs/05-api.md`.

Repo cũng có sẵn collection dạng v3 (`postman/collections/`) đã khai báo đủ cả WebSocket và gRPC,
chạy bằng Postman CLI: `postman collection run "postman/collections/Chat Service"`.
"""

collection = {
    "info": {
        "_postman_id": COLLECTION_ID,
        "name": "Chat Service · ABC Workspace",
        "description": DESCRIPTION,
        "schema": SCHEMA,
    },
    "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "{{token}}", "type": "string"}]},
    "event": [
        {
            "listen": "prerequest",
            "script": {
                "type": "text/javascript",
                "exec": [
                    "// Bot API dùng service key thay vì JWT -> tự gắn header.",
                    "if (pm.request.url.getPath().startsWith('/bot/v1')) {",
                    "  if (!pm.request.headers.has('X-Chat-Service-Key')) {",
                    "    pm.request.headers.add({key: 'X-Chat-Service-Key', value: pm.variables.get('service_key') || ''});",
                    "  }",
                    "  pm.request.headers.upsert({key: 'Idempotency-Key', value: pm.variables.replaceIn('{{$guid}}')});",
                    "}",
                    "// Cảnh báo sớm nếu chưa cấu hình biến bắt buộc.",
                    'const need = ["base_url", "token"];',
                    "need.forEach(k => {",
                    "  if (!pm.variables.get(k)) {",
                    '    console.warn("Thiếu biến " + k + " — chọn environment \\"Chat Service · Local\\" và dán token (make token).");',
                    "  }",
                    "});",
                ],
            },
        }
    ],
    "variable": [
        {"key": "base_url", "value": "http://localhost:8080", "type": "string"},
        {"key": "token", "value": "", "type": "string"},
        {"key": "bot_slug", "value": "bot", "type": "string"},
        {"key": "service_key", "value": "", "type": "string"},
        {"key": "cms_token", "value": "", "type": "string"},
        {"key": "chat_id", "value": "", "type": "string"},
        {"key": "actor_id", "value": "", "type": "string"},
        {"key": "card_message_id", "value": "", "type": "string"},
        {"key": "user_id", "value": "", "type": "string"},
        {"key": "user_id_2", "value": "", "type": "string"},
        {"key": "conversation_id", "value": "", "type": "string"},
        {"key": "direct_conversation_id", "value": "", "type": "string"},
        {"key": "message_id", "value": "", "type": "string"},
        {"key": "thread_id", "value": "", "type": "string"},
        {"key": "team_id", "value": "", "type": "string"},
        {"key": "task_id", "value": "", "type": "string"},
        {"key": "task_code", "value": "", "type": "string"},
        {"key": "assignment_id", "value": "", "type": "string"},
        {"key": "revision_id", "value": "", "type": "string"},
        {"key": "reply_message_id", "value": "", "type": "string"},
    ],
    "item": [start, conversations, messages, teams, bots, tasks, revisions, errors,
             cms_bot, cms_client, cleanup],
}

environment = {
    "id": ENV_ID,
    "name": "Chat Service · Local",
    "values": [
        {"key": "base_url", "value": "http://localhost:8080", "type": "default", "enabled": True},
        {"key": "grpc_url", "value": "localhost:9090", "type": "default", "enabled": True},
        {"key": "ws_url", "value": "ws://localhost:8080/ws", "type": "default", "enabled": True},
        {"key": "token", "value": "", "type": "secret", "enabled": True},
        {"key": "token_2", "value": "", "type": "secret", "enabled": True},
        {"key": "cms_token", "value": "", "type": "secret", "enabled": True},
        {"key": "service_key", "value": "", "type": "secret", "enabled": True},
        {"key": "chat_id", "value": "", "type": "default", "enabled": True},
        {"key": "user_id_2", "value": "", "type": "default", "enabled": True},
    ],
    "_postman_variable_scope": "environment",
}


V3_DIR = OUT_DIR / "collections" / "chat-service"
# Folder viet tay trong ban v3 (v2.1 khong chua duoc 2 protocol nay) — khong ghi de.
HANDWRITTEN = ("08 · WebSocket", "09 · gRPC")


def sync_v3():
    """Dich file v2.1 sang collection v3 (de `postman collection run` + review trong git).

    Chi thay cac folder HTTP do script sinh ra; folder WebSocket/gRPC viet tay giu nguyen.
    """
    import shutil
    import subprocess
    import tempfile

    if shutil.which("postman") is None:
        print("! khong thay Postman CLI → bo qua ban v3 (chi sinh file v2.1)")
        return
    with tempfile.TemporaryDirectory() as tmp:
        out = pathlib.Path(tmp) / "chat-service"
        res = subprocess.run(
            ["postman", "collection", "migrate", str(COLLECTION_FILE), "-o", str(out)],
            capture_output=True, text=True,
        )
        if res.returncode != 0:
            print("! postman collection migrate loi:", res.stderr.strip()[:200])
            return
        V3_DIR.mkdir(parents=True, exist_ok=True)
        for item in V3_DIR.iterdir():
            if item.name in HANDWRITTEN:
                continue
            shutil.rmtree(item) if item.is_dir() else item.unlink()
        for item in out.iterdir():
            dst = V3_DIR / item.name
            shutil.copytree(item, dst) if item.is_dir() else shutil.copy2(item, dst)
    print(f"✓ {V3_DIR.relative_to(OUT_DIR.parent)}/ (v3 cho Postman CLI, giu {', '.join(HANDWRITTEN)})")


def main():
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    COLLECTION_FILE.write_text(json.dumps(collection, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    ENV_FILE.write_text(json.dumps(environment, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    n = sum(len(f["item"]) for f in collection["item"])
    print(f"✓ {COLLECTION_FILE.relative_to(OUT_DIR.parent)} ({len(collection['item'])} folder, {n} request)")
    print(f"✓ {ENV_FILE.relative_to(OUT_DIR.parent)}")
    sync_v3()


if __name__ == "__main__":
    main()
