#!/usr/bin/env python3
"""Sinh Postman collection (schema v2.1) + environment cho chat-service.

Chay lai moi khi doi API:  make postman
Nguon su that van la code/proto; file nay chi dich sang dang Postman import duoc.
"""
import json
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
# Chi sinh 2 file JSON (collection v2.1 + environment). KHONG sinh cay v3
# `collections/` nua — xem docs/backend.md#postman.
OUT_DIR = ROOT / "internal" / "postman"
COLLECTION_FILE = OUT_DIR / "chat-service.postman_collection.json"
ENV_FILE = OUT_DIR / "chat-service.local.postman_environment.json"
# Hai ban tach de gui cho tung doi: moi ban chi co phan ben do goi + FLOW rieng.
FE_FILE = OUT_DIR / "chat-service-fe.postman_collection.json"
CMS_FILE = OUT_DIR / "chat-service-cms.postman_collection.json"

SCHEMA = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
# ID on dinh de import nhieu lan khong tao ban trung.
COLLECTION_ID = "6f1a0c54-3c9f-4f0e-9a2a-1b7c5d0e1001"
ENV_ID = "6f1a0c54-3c9f-4f0e-9a2a-1b7c5d0e2001"
FE_ID = "6f1a0c54-3c9f-4f0e-9a2a-1b7c5d0e1002"
CMS_ID = "6f1a0c54-3c9f-4f0e-9a2a-1b7c5d0e1003"

# values mau cua form create_task (FE gui, Chat chuyen nguyen sang CMS).
# Them truong moi vao form: sua o day + docs/frontend.md, khong phai sua Go.
CREATE_TASK_VALUES_SAMPLE = {
    "kind": "task",
    "teamId": "27",
    "normItemId": "74",
    "title": "Banner Tết 2027",
    "brief": "Tông đỏ vàng, có logo",
    "quantity": "2",
    "size": "1200x628",
    "urgencyCode": "normal",
    "useAI": False,
}


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
        # Moi phan tu: (key, value) hoac (key, value, disabled).
        # disabled=True => tham so hien trong Postman nhung tat san (filter tuy chon).
        norm = [(q[0], q[1], (len(q) > 2 and q[2])) for q in query]
        url["query"] = [
            {"key": k, "value": v, **({"disabled": True} if off else {})}
            for k, v, off in norm
        ]
        qs = "&".join(f"{k}={v}" for k, v, off in norm if not off)
        url["raw"] = "{{base_url}}" + clean_path(path) + ("?" + qs if qs else "")
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
        req("Tài liệu FLOW.md (markdown)", "GET", "/docs/flow",
            headers={"X-Docs-Key": "{{docs_key}}"},
            auth={"type": "noauth"},
            desc="Trả nguyên văn `docs/FLOW.md` (`text/markdown`) để dựng trang tài liệu. "
                 "Auth: header `X-Docs-Key` = `DOCS_API_KEY` trong `.env` (biến `docs_key`), "
                 "không dùng JWT.\n\n"
                 "- sai/thiếu khoá → `401 invalid_docs_key`\n"
                 "- chưa đặt `DOCS_API_KEY` → route không tồn tại (`404`)\n\n"
                 "Nội dung được nhúng vào binary lúc build: sửa `FLOW.md` xong phải build/deploy lại.",
            tests=('pm.test("200 (dung khoa) / 401 (sai khoa) / 404 (chua bat)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 401, 404]));',)),
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
    "Chat nhóm và chat riêng. `Tạo nhóm` lưu `group_id` — mọi folder sau đều dùng biến này.",
    [
        req("Tạo nhóm", "POST", "/api/v1/conversations",
            body={"type": "group", "name": "Tổ Design · Đơn hàng 11",
                  "member_user_ids": []},
            desc="`type=group` bắt buộc có `name`. Người tạo là `owner`, người được thêm là `member`.",
            tests=(OK, *save("group_id", "_d.id"),
                   'pm.test("la group", () => pm.expect(pm.response.json().data.type).to.eql("group"));')),
        req("Tạo chat riêng (1-1)", "POST", "/api/v1/conversations",
            body={"type": "direct", "member_user_ids": ["{{user_id_2}}"]},
            desc="Cần `user_id_2` (gọi `/me` bằng token người thứ 2). Gọi 2 lần trả về **cùng** "
                 "một hội thoại (chống tạo trùng). Nếu `user_id_2` = chính mình → lỗi `invalid_direct_members`.",
            tests=('pm.test("200 hoặc 400 nếu chưa có user thứ 2",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 400]));',
                   'if (pm.response.code < 300) pm.collectionVariables.set("direct_conversation_id", pm.response.json().data.id);')),
        req("Danh sách hội thoại", "GET", "/api/v1/conversations",
            query=[("limit", "20"),
                   ("type", "group", True), ("source", "cms", True), ("q", "TEAM", True)],
            desc="Mới nhất trước, kèm `unread_count`. Trang sau: thêm `cursor` = `meta.next_cursor`.\n\n"
                 "**Filter** (bật trong tab Params, bỏ trống = không lọc):\n"
                 "- `type` — `direct` | `group`. Giá trị khác → 400 `invalid_type`.\n"
                 "- `source` — `cms` | `local`. Giá trị khác → 400 `invalid_source`.\n"
                 "- `q` — tìm trong tên nhóm, không phân biệt hoa thường; `%` và `_` được escape.",
            tests=(OK, 'pm.test("co meta", () => pm.expect(pm.response.json()).to.have.property("meta"));')),
        req("Danh sách hội thoại — lọc sai type", "GET", "/api/v1/conversations",
            query=[("type", "nhom")],
            desc="Giá trị `type` ngoài `direct|group` bị từ chối ngay thay vì trả danh sách rỗng khó hiểu.",
            tests=('pm.test("400 invalid_type", () => { pm.response.to.have.status(400);'
                   ' pm.expect(pm.response.json().error.code).to.eql("invalid_type"); });',)),
        req("Chi tiết hội thoại", "GET", "/api/v1/conversations/{{group_id}}",
            desc="Kèm danh sách thành viên và role (owner/admin/member)."),
        req("Thêm thành viên (join room)", "POST", "/api/v1/conversations/{{group_id}}/members",
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
        req("Gửi tin nhắn text", "POST", "/api/v1/conversations/{{group_id}}/messages",
            body={"type": "text", "content": "Chào cả nhà 👋", "client_msg_id": "{{$guid}}"},
            desc="`client_msg_id` do FE sinh và phải **duy nhất mỗi tin**: gửi lại đúng id cũ sẽ "
                 "trả về tin cũ với `duplicated=true`, **không tạo tin mới và không broadcast**. "
                 "Ở đây dùng `{{$guid}}` nên mỗi lần Send là một id mới.",
            tests=(OK, *save("message_id", "_d.message.id"),
                   'pm.collectionVariables.set("thread_id", pm.response.json().data.message.id);')),
        req("Gửi kèm ảnh/file", "POST", "/api/v1/conversations/{{group_id}}/messages",
            body={"type": "image", "content": "Mẫu banner v1",
                  "attachments": [{"kind": "image", "url": "https://picsum.photos/800/600",
                                   "name": "banner-v1.png", "mime_type": "image/png",
                                   "size_bytes": 102400, "width": 800, "height": 600}],
                  "client_msg_id": "{{$guid}}"},
            desc="Service chỉ lưu URL — việc upload do service file/CDN khác làm.",
            tests=(OK,
                   'const m = pm.response.json().data.message;',
                   'if (m.attachments && m.attachments[0]) pm.collectionVariables.set("attachment_id", m.attachments[0].id);',
                   'pm.test("co attachment", () => pm.expect(m.attachments).to.have.lengthOf(1));')),
        req("Trả lời trong thread", "POST", "/api/v1/conversations/{{group_id}}/messages",
            body={"content": "Phản hồi trong thread của nhiệm vụ", "thread_id": "{{thread_id}}",
                  "client_msg_id": "{{$guid}}"},
            desc="`thread_id` = id tin nhắn gốc (hoặc card task). Phản hồi hiện cả trong nhóm và trong thread."),
        req("Trả lời (trích dẫn) một tin nhắn", "POST", "/api/v1/conversations/{{group_id}}/messages",
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
        req("Lịch sử tin nhắn", "GET", "/api/v1/conversations/{{group_id}}/messages",
            query=[("limit", "30")],
            desc="Mới nhất trước. Dùng `cursor` = `meta.next_cursor` để tải tiếp (đừng dùng offset).\n\n"
                 "Mỗi tin kèm `reply_count` (trả lời trực tiếp) và `thread_count` (số tin trong thread "
                 "tin đó mở) để UI hiện badge \"N phản hồi\" mà không phải gọi thêm API.",
            tests=(OK, 'pm.test("co mang data", () => pm.expect(pm.response.json().data).to.be.an("array"));')),
        req("Tin nhắn trong 1 thread", "GET", "/api/v1/conversations/{{group_id}}/messages",
            query=[("thread_id", "{{thread_id}}"), ("limit", "50")],
            desc="Trả cả tin nhắn gốc + mọi phản hồi trong thread."),
        req("Tìm tin nhắn", "GET", "/api/v1/messages/search",
            query=[("q", "banner"), ("conversation_id", "{{group_id}}"), ("limit", "20")],
            desc="Tìm theo nội dung, mã task (`metadata.task_code`), người gửi (`sender_id`), "
                 "khoảng ngày (`from_date`, `to_date` dạng YYYY-MM-DD)."),
        req("Đánh dấu đã đọc", "POST", "/api/v1/conversations/{{group_id}}/read",
            body={"message_id": "{{message_id}}"},
            desc="Trả về `unread_count` còn lại. Mốc đã đọc chỉ tiến, không lùi.",
            tests=(OK, 'pm.test("co unread_count", () => pm.expect(pm.response.json().data).to.have.property("unread_count"));')),
        req("Ai đã xem tin này", "GET", "/api/v1/messages/{{message_id}}/readers",
            desc="Danh sách người đã đọc (bảng `message_receipts`)."),
        req("Ghim tin nhắn", "POST", "/api/v1/messages/{{message_id}}/pin",
            body={},
            desc="Ghim tin trong nhóm. Gửi `{\"pinned\": false}` để bỏ ghim. Ghim lại tin đã ghim "
                 "không đổi gì (idempotent). Cả nhóm nhận event `message` để cập nhật.",
            tests=(OK, 'pm.test("da ghim", () => pm.expect(pm.response.json().data.pinned).to.be.true);')),
        req("Danh sách tin đã ghim", "GET", "/api/v1/conversations/{{group_id}}/pinned",
            query=[("limit", "50")],
            desc="Mới ghim trước.",
            tests=(OK, 'pm.test("la mang", () => pm.expect(pm.response.json().data).to.be.an("array"));')),
        req("Bỏ ghim", "POST", "/api/v1/messages/{{message_id}}/pin",
            body={"pinned": False},
            tests=(OK, 'pm.test("da bo ghim", () => pm.expect(pm.response.json().data.pinned).to.be.false);')),
        req("Xoá tin nhắn (mềm)", "DELETE", "/api/v1/messages/{{reply_message_id}}",
            desc="Người gửi hoặc owner/admin nhóm. Xoá **tin trả lời** vừa tạo ở trên, không xoá "
                 "`message_id` để các folder sau còn dùng được.",
            tests=('pm.test("200 hoặc 403", () => pm.expect(pm.response.code).to.be.oneOf([200, 403]));',)),
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
        req("@bot (không có mẫu → help)", "POST", "/api/v1/conversations/{{group_id}}/messages",
            body={"content": "@bot", "client_msg_id": "{{$guid}}"},
            desc="Bot trả về danh sách mẫu đang có.",
            tests=(OK, 'pm.test("bot tra loi", () => pm.expect(pm.response.json().data.bot_reply).to.not.eql(null));')),
        req("@bot yeu_cau → tạo task", "POST", "/api/v1/conversations/{{group_id}}/messages",
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
            body={"conversation_id": "{{group_id}}", "template_code": "phieu_sua",
                  "payload": {"ma_task": "T-000001", "nguyen_nhan": "Sai màu nền",
                              "loai_sua": "màu", "so_luong": 2},
                  "client_msg_id": "{{$guid}}"},
            desc="Dùng khi FE có form riêng thay vì gõ `@bot`.\n\n"
                 "`ma_task` ở đây chỉ là giá trị mẫu. Khi `TASK_ENGINE_ENABLED=false` (cấu hình "
                 "chuẩn lúc tích hợp CMS) Chat **chỉ lưu** mẫu với `status=stored`, không tạo phiếu "
                 "sửa — đơn/task do CMS sở hữu.",
            tests=('pm.test("200/201", () => pm.expect(pm.response.code).to.be.oneOf([200, 201]));',)),
        req("Các mẫu đã lưu", "GET", "/api/v1/bots/{{bot_slug}}/submissions",
            query=[("conversation_id", "{{group_id}}"), ("limit", "20")],
            desc="Lọc thêm `status` = stored | processing | done | failed."),
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
        req("ID nhóm không tồn tại → 404", "GET", "/api/v1/conversations/khong-phai-uuid",
            desc="`:id` của route hội thoại nhận **cả UUID lẫn chatId của CMS** (`wg_<hex>`), nên chuỗi "
                 "không phải UUID không còn bị coi là sai định dạng — nó được tra như một `external_id`. "
                 "Không có thì `404 conversation_not_found`.",
            tests=('pm.test("404", () => pm.response.to.have.status(404));',
                   'pm.test("code conversation_not_found", () => pm.expect(pm.response.json().error.code)'
                   '.to.eql("conversation_not_found"));')),
        req("Hội thoại không tồn tại → 403/404", "GET",
            "/api/v1/conversations/00000000-0000-0000-0000-000000000000/messages",
            desc="Không phải thành viên → `not_conversation_member` (không tiết lộ hội thoại có tồn tại hay không).",
            tests=('pm.test("403 hoặc 404", () => pm.expect(pm.response.code).to.be.oneOf([403, 404]));',)),
        req("Endpoint không tồn tại → 404", "GET", "/api/v1/khong-co-endpoint-nay",
            desc="Code: `route_not_found`. Lỗi của Fiber (404/405/413/upgrade) được map sang đúng "
                 "status thay vì 500.",
            tests=('pm.test("404", () => pm.response.to.have.status(404));',
                   'pm.test("code route_not_found", () => pm.expect(pm.response.json().error.code).to.eql("route_not_found"));')),
        req("Tin nhắn rỗng → 400", "POST", "/api/v1/conversations/{{group_id}}/messages",
            body={"content": "   "},
            tests=('pm.test("400", () => pm.response.to.have.status(400));',
                   'pm.test("code empty_message", () => pm.expect(pm.response.json().error.code).to.eql("empty_message"));')),
    ],
)

cms_bot = folder(
    "08 · CMS → Chat (Bot API)",
    "Các API **CMS gọi sang Chat**. Auth: header `X-Chat-Service-Key` (biến `service_key`), "
    "không dùng JWT. Mọi request ghi nên có `Idempotency-Key` — gặp lại key trả nguyên kết quả cũ.\n\n"
    "Chỉ bật khi `CMS_ENABLED=true`. Chi tiết: `docs/backend.md#tích-hợp-cms-designer-bot`.",
    [
        req("Resolve user (openId → actorId)", "POST", "/bot/v1/users/resolve",
            body={"items": [{"openId": "local:designer-1", "cmsUserId": 68, "name": "Trần Designer"}]},
            desc="Chưa có trong Chat thì tạo actor mới (lấy hồ sơ qua Workspace API của CMS).",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co actorId", () => pm.expect(pm.response.json().items[0]).to.have.property("actorId"));',
                   'const it = pm.response.json().items[0];',
                   'if (it) pm.collectionVariables.set("actor_id", it.actorId);')),
        req("Gửi card vào nhóm", "POST", "/bot/v1/messages",
            body={"groupId": "{{group_id}}", "bot": {"id": 1, "name": "Demo Designer Bot"},
                  "card": {"schema": "abc.card/v1",
                           "header": {"code": "WEB-20261118-0006", "kind": "DESIGN",
                                      "title": "Banner Tết 2027",
                                      "status": {"text": "Chưa nhận", "tone": "warning"}},
                           "visibleTo": None,
                           "elements": [
                               {"type": "mentions", "items": [
                                   {"role": "code", "text": "WEB-20261118-0006"},
                                   {"role": "assignee", "actor": "{{actor_id}}", "name": "Trần Designer",
                                    "username": "des3"}]},
                               {"type": "button", "text": "Nhận task",
                                "action": "accept_task", "visibleTo": ["{{actor_id}}"]}]},
                  "mentions": ["{{actor_id}}"]},
            desc="`groupId` nhận cả `wg_…` của CMS lẫn UUID nội bộ. Trả `{messageId, cardVersion}`.\n\n"
                 "`mentions` (tuỳ chọn) = actorId được tag trên card → Chat đánh dấu \"@ bạn\" "
                 "(`mentionCount` của nhóm) cho tới khi họ đọc. Chat **không** gửi thông báo cho "
                 "những người này (CMS đã gửi qua `/bot/v1/notifications`). Người ngoài nhóm bị bỏ qua. "
                 "`POST /bot/v1/messages/:id/thread` nhận cùng field.",
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
            desc="Chỉ áp dụng khi `cardVersion` **lớn hơn** bản đang lưu; nhỏ hơn/bằng → `{\"ok\":true,\"ignored\":true}`.\n\n"
                 "Gộp được 3 việc trong 1 request — xem request kế tiếp.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("ok", () => pm.expect(pm.response.json().ok).to.be.true);')),
        req("Sửa card + thread + thông báo (gộp 1 request)", "PATCH",
            "/bot/v1/messages/{{card_message_id}}",
            body={"card": {"schema": "abc.card/v1",
                           "header": {"code": "WEB-20261118-0006", "title": "Banner Tết 2027",
                                      "status": {"text": "Đã nộp bài", "tone": "success"}},
                           "elements": [{"type": "text", "text": "Chờ duyệt"}]},
                  "cardVersion": 3,
                  "thread": {"text": "Trần Designer đã nộp bài · Chờ duyệt",
                             "bot": {"id": 1, "name": "Demo Designer Bot"}},
                  "notify": [{"actorId": "{{actor_id}}",
                              "text": "Đơn WEB-20261118-0006 đã nộp bài"}]},
            desc="**Dùng cái này cho mỗi lần đơn đổi trạng thái** — thay vì gọi 3 request "
                 "(`PATCH` card + `POST .../thread` + `POST /notifications`).\n\n"
                 "`thread` và `notify` đều **tuỳ chọn**; không gửi thì hành vi y như request ở trên.\n"
                 "`notify[].messageId` bỏ trống thì tự trỏ về chính card này.\n\n"
                 "**Quy ước lỗi** (đã kiểm chứng):\n"
                 "- `cardVersion` không lớn hơn → `{ok, ignored:true}` và **bỏ qua cả** `thread`/`notify` "
                 "(đang gửi trạng thái cũ thì không được ghi thêm dòng diễn biến cũ).\n"
                 "- `thread`/`notify` lỗi → vẫn `200`, card đã đổi xong, lỗi báo theo từng phần: "
                 "`{\"thread\":{\"error\":{\"code\":\"text_required\"}}}`. Không trả 5xx vì CMS retry "
                 "sẽ gặp `ignored:true` (cardVersion không tăng nữa) và mất luôn thread/notify.\n"
                 "- Một `Idempotency-Key` cho cả 3 việc.",
            auth={"type": "noauth"},
            tests=(OK,
                   'const d = pm.response.json();',
                   'pm.test("co cardVersion", () => pm.expect(d).to.have.property("cardVersion"));',
                   'pm.test("ignored thi khong co thread/notify", () => {',
                   '  if (d.ignored) { pm.expect(d).to.not.have.property("thread");',
                   '                   pm.expect(d).to.not.have.property("notify"); }',
                   '});')),
        req("Trả lời trong thread", "POST", "/bot/v1/messages/{{card_message_id}}/thread",
            body={"text": "Đã giao Trần Designer · Chờ nhận task.", "bot": {"id": 1, "name": "Demo Designer Bot"}},
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co messageId", () => pm.expect(pm.response.json()).to.have.property("messageId"));')),
        req("Card vào thread (chia task)", "POST", "/bot/v1/messages/{{card_message_id}}/thread",
            body={"bot": {"id": 1, "name": "Designer Bot"},
                  "card": {"schema": "abc.card/v1",
                           "header": {"code": "WEB-20261120-0003-P2", "title": "Banner Tết · Phần 2/3",
                                      "status": {"text": "Chưa nhận", "tone": "warning"}},
                           "elements": [{"type": "actions", "items": [
                               {"action": "accept_task", "label": "Nhận task", "placement": "primary",
                                "visibleTo": ["{{actor_id}}"], "confirm": {"title": "Nhận phần 2/3?"}}]}]}},
            auth={"type": "noauth"},
            desc="Chia task: card của phần 2..N nằm **trong thread của card gốc**. Body cần `text` **hoặc** `card`.\n\n"
                 "Trả `messageId` + `cardVersion` (không gửi thì bắt đầu từ `1`) + `threadRootId`. Card trong "
                 "thread dùng được như card ở timeline: `PATCH /bot/v1/messages/:id`, lọc `visibleTo`, "
                 "FE bấm nút → `card.action` với `messageId` = id của card phần.",
            tests=(OK, 'const d = pm.response.json();',
                   'pm.test("co messageId + cardVersion + threadRootId",'
                   ' () => pm.expect(d).to.include.keys("messageId", "cardVersion", "threadRootId"));')),
        req("Thông báo cho 1 người", "POST", "/bot/v1/notifications",
            body={"actorId": "{{actor_id}}", "text": "Bạn có task mới: WEB-20261118-0006",
                  "messageId": "{{card_message_id}}"},
            auth={"type": "noauth"},
            tests=(OK,)),
        req("Đẩy nhóm + thành viên", "PUT", "/bot/v1/groups/{{cms_chat_id}}",
            body={"name": "Nhóm Thiết kế Web", "avatarUrl": "/api/avatars/group-web.png",
                  "members": [{"openId": "local:designer-1", "cmsUserId": 68, "name": "Trần Designer",
                               "sources": ["team"], "teamCodes": ["DESIGN"]}]},
            desc="**Thay thế toàn bộ** danh sách thành viên: ai không còn trong danh sách mất quyền xem nhóm ngay.\n\n"
                 "Field nhóm (`type`, `avatarUrl`, `botId`, `teams`…) đều tuỳ chọn — **không gửi = giữ nguyên**. "
                 "`avatarUrl: null` = gỡ avatar nhóm.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co memberCount", () => pm.expect(pm.response.json()).to.have.property("memberCount"));')),
        req("Tạo chat riêng với bot (dm)", "PUT", "/bot/v1/groups/dm_{{actor_id}}",
            body={"type": "dm", "name": "Designer Bot", "avatarUrl": "/api/avatars/bot.png", "botId": 1,
                  "members": [{"openId": "local:designer-1", "cmsUserId": 68, "name": "Trần Designer"}]},
            desc="`type: \"dm\"` = chat riêng giữa bot và **1 người** (nhắc nhận task, quá hạn, Drive…). "
                 "Bot gửi tin vào đó bằng `POST /bot/v1/messages` với `groupId` = chatId này.\n\n"
                 "Người dùng **không** gửi tin / form vào nhóm dm được: `403 BOT_DM_READ_ONLY`. "
                 "FE thấy `bootstrap.groups[].type = \"dm\"` để ẩn ô nhập.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("type = dm", () => pm.expect(pm.response.json().type).to.eql("dm"));')),
        req("Ghim tin nhắn", "POST", "/bot/v1/messages/{{card_message_id}}/pin",
            body={},
            desc="CMS ghim card/tin quan trọng. `{\"pinned\": false}` để bỏ ghim.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("ok", () => pm.expect(pm.response.json().ok).to.be.true);')),
        req("Cập nhật presence (lịch OFF)", "PUT", "/bot/v1/presence",
            body={"items": [
                {"actorId": "{{actor_id}}", "status": "off", "reason": "Lịch OFF ca chiều"},
            ]},
            desc="`status`: `on` → online; `off` **kèm `reason`** → `off_duty` (hiện chữ OFF cạnh tên); "
                 "`off` không reason → offline. Actor không tồn tại thì bị bỏ qua, không lỗi.\n\n"
                 "Chat phát event realtime `presence` cho những người **chung hội thoại** với người đó.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co so dong cap nhat", () => pm.expect(pm.response.json().updated).to.be.a("number"));')),
        req("Tra cứu: danh sách nhóm", "GET", "/bot/v1/groups",
            query=[("q", "TEAM", True), ("source", "cms", True)],
            desc="Spec §7.7. Liệt kê nhóm Chat đang giữ — dùng để đối chiếu khi debug đồng bộ.\n\n"
                 "**Filter** (bật trong tab Params): `q` tìm theo tên (không phân biệt hoa thường), "
                 "`source` = `cms` | `local`.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co items", () => pm.expect(pm.response.json()).to.have.property("items"));')),
        req("Tra cứu: 1 nhóm", "GET", "/bot/v1/groups/{{group_id}}",
            desc="Spec §7.7. Nhận `chatId` của CMS hoặc UUID nội bộ.",
            auth={"type": "noauth"},
            tests=('pm.test("200 / 404", () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',)),
        req("Tra cứu: 1 user", "GET", "/bot/v1/users/{{actor_id}}",
            desc="Spec §7.7. Nhận `actorId` (`u_68`), `openId`, hoặc UUID nội bộ.",
            auth={"type": "noauth"},
            tests=('pm.test("200 / 404", () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',)),
        req("Tra cứu: nhiều user", "POST", "/bot/v1/users/batch",
            body={"actorIds": ["{{actor_id}}"], "openIds": [], "cmsUserIds": []},
            desc="Spec §7.7. Tra hàng loạt bằng **một** query cho mỗi loại id (không phải N query).",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co items", () => pm.expect(pm.response.json()).to.have.property("items"));')),
        req("Thông tin tệp đính kèm", "GET", "/bot/v1/attachments/{{attachment_id}}",
            desc="Chat **không lưu file**, chỉ lưu URL do service upload/CDN cung cấp. "
                 "Thêm `?redirect=true` để nhảy thẳng tới URL đó (302).",
            auth={"type": "noauth"},
            tests=('pm.test("200 hoac 404 neu chua co attachment_id",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 404]));',)),
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
    "khi `AUTH_ALLOW_DEV_TOKEN=true`.\n\n"
    "**`{{group_id}}` là gì**: id nhóm trong URL — một biến duy nhất cho cả 3 bề mặt. "
    "Nhận **UUID nội bộ** hoặc **`wg_<hex>` (chatId của CMS)**, endpoint tự nhận ra. "
    "Chạy folder `FLOW` hoặc `01 · Hội thoại` trước là biến này có sẵn.\n\n"
    "Khác với `/api/v1`: envelope `{statusCode, code, message, element}` và field dạng camelCase "
    "(`text`, `replyToId`, `threadRootId`, `clientMsgId`).",
    [
        req("Bootstrap", "GET", "/api/client/v1/chat/bootstrap",
            desc="Tất cả thứ FE cần khi mở app: `me`, `groups` (chỉ nhóm mình là thành viên, "
                 "kèm `members` + `sources`/`teamCodes`), `pinnedChats` (nhóm mình đã ghim, ghim gần nhất "
                 "trước), `chats` (thanh bên kiểu Lark: nhóm + thread trộn chung, trang đầu, **không gồm "
                 "nhóm đã ghim**) + `chatsPageInfo`, `notifications`, `unreadNotifications`.\n\n"
                 "Mỗi `groups[]` có thêm: `type` (`group` | `dm` = chat riêng với bot → ẩn ô nhập), "
                 "`avatarUrl` (avatar nhóm từ CMS, rỗng = chưa có), `unreadCount`, `mentionCount` "
                 "(tin chưa đọc nhắc tới mình → dấu \"@\"), `canMentionAll` (hiện dòng `@all`), "
                 "`pinned` / `pinnedAt`. `members[]` có `actorId`, `name`, `username`, `avatarUrl` — "
                 "dùng cho popup gợi ý khi gõ `@`.\n\n"
                 "Trang sau: `?chatCursor=<chatsPageInfo.nextCursor>` (xem request lazy load).",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK, 'const el = pm.response.json().element;',
                   'pm.test("co me + groups", () => pm.expect(el).to.include.all.keys(["me","groups","pinnedChats","chats","chatsPageInfo","notifications","unreadNotifications"]));',
                   'if (el.groups && el.groups[0]) pm.collectionVariables.set("group_id", el.groups[0].chatId || el.groups[0].id);',
                   'pm.collectionVariables.set("chat_cursor", (el.chatsPageInfo || {}).nextCursor || "");')),
        req("Tôi là ai", "GET", "/api/client/v1/chat/auth/me",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Đồng bộ lại nhóm từ CMS", "POST", "/api/client/v1/chat/groups/refresh",
            desc="Gọi sau khi user vào nhóm bằng link mời.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (OK) / 400 (chua bat CMS) / 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
        req("Lịch sử tin nhắn (card đã lọc)", "GET", "/api/client/v1/chat/groups/{{group_id}}/messages",
            query=[("limit", "30"), ("threadRootId", "{{card_message_id}}", True),
                   ("includeThread", "true", True)],
            desc="Card trả về đã **lọc nút theo `visibleTo`** của chính người gọi; `actions` liệt kê "
                 "action người đó được bấm.\n\n"
                 "**Mặc định chỉ trả tin gốc** — tin trả lời trong thread gom vào tin gốc (`threadCount`). "
                 "Bật `threadRootId` để mở một thread (tin gốc + các tin trả lời); bật `includeThread=true` "
                 "để lấy cả tin trong thread lẫn vào lịch sử (hành vi cũ).\n\n⚠️ `cms_token` phải là người **thuộc nhóm** `group_id`, "
                 "không thì `403 not_conversation_member`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (token thuoc nhom) hoac 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 403]));',)),
        req("Gửi tin nhắn", "POST", "/api/client/v1/chat/groups/{{group_id}}/messages",
            body={"text": "Nhờ team làm gấp giúp mình nhé", "clientMsgId": "{{$guid}}"},
            desc="Chat lưu + phát realtime + gửi `message.created` sang CMS (bất đồng bộ qua outbox).",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (token thuoc nhom) hoac 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 403]));',)),
        req("Ghim nhóm", "POST", "/api/client/v1/chat/groups/{{group_id}}/pin",
            body={"pinned": True},
            desc="Ghim nhóm lên đầu thanh bên — **riêng người gọi**, người khác không bị ảnh hưởng. "
                 "`{\"pinned\": false}` = bỏ ghim; body rỗng = ghim. Ghim lại nhóm đã ghim = đưa lên trên cùng.\n\n"
                 "Sau đó `bootstrap` trả nhóm trong `pinnedChats` (ghim gần nhất trước) và **bỏ** nó khỏi "
                 "`chats`; `groups[].pinned = true`. Các tab khác của chính người đó nhận `reload` "
                 "`{reason: \"group.pinned\"}`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (OK) / 403 (khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 403]));',)),
        req("Bỏ ghim nhóm", "POST", "/api/client/v1/chat/groups/{{group_id}}/pin",
            body={"pinned": False},
            desc="Bỏ ghim — nhóm quay về `chats`, xếp theo hoạt động gần nhất.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (OK) / 403 (khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 403]));',)),
        req("Gửi tin nhắc tên (@)", "POST", "/api/client/v1/chat/groups/{{group_id}}/messages",
            body={"text": "Nhờ @Trần Designer xem giúp", "mentions": ["{{actor_id}}"],
                  "mentionAll": False, "clientMsgId": "{{$guid}}"},
            desc="`mentions` = actorId người được nhắc: Chat chỉ giữ thành viên nhóm, bỏ người gửi, "
                 "bỏ trùng (tối đa 100), rồi gửi mỗi người 1 thông báo "
                 "\"<Người gửi> nhắc đến bạn trong <nhóm>: …\".\n\n"
                 "`mentionAll: true` = @all — chỉ admin / trợ lý của nhóm; người khác nhận "
                 "`403 MENTION_ALL_FORBIDDEN`. Tin trả về có `mentions`, `mentionAll`, `mentionedMe`; "
                 "`message.created` sang CMS cũng kèm `mentions`/`mentionAll`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (OK) / 403 (khong thuoc nhom / khong duoc @all)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 403]));',)),
        req("Phản hồi của 1 tin nhắn", "GET", "/api/client/v1/chat/messages/{{card_message_id}}/replies",
            query=[("mode", "thread"), ("limit", "50")],
            desc="Trả `root`, `replies` (card đã lọc `visibleTo`), `replyCount`, `threadCount`. "
                 "`mode=direct` cho trích dẫn, `mode=thread` cho panel thread của card.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (token thuoc nhom) hoac 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 403]));',)),
        req("Bootstrap · trang thanh bên tiếp (lazy load)", "GET", "/api/client/v1/chat/bootstrap",
            query=[("chatCursor", "{{chat_cursor}}"), ("chatLimit", "20")],
            desc="Thanh bên kiểu Lark: `chats` = **nhóm và thread trộn chung**, hoạt động gần nhất trước. "
                 "Trang đầu nằm sẵn trong `bootstrap`; cuộn xuống thì gọi lại với "
                 "`chatCursor = chatsPageInfo.nextCursor`.\n\n"
                 "Có `chatCursor` → **chỉ trả `chats` + `chatsPageInfo`**, không tải lại nhóm / thông báo.\n\n"
                 "Mỗi dòng: `type` (`group`|`thread`), `id`, `title`, `preview` (\"Người gửi: nội dung\"), "
                 "`lastActivityAt`, `lastMessage`, `group` (có `type`, `avatarUrl`). Nhóm thêm "
                 "`unreadCount`, `mentionCount`, `pinned`, `pinnedAt`; thread thêm `root`, `threadCount`, "
                 "`replyCount`.\n\nNhóm đã ghim không có trong `chats` — chúng nằm ở `pinnedChats` của "
                 "`bootstrap` trang đầu.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK, 'const el = pm.response.json().element;',
                   'pm.test("chats la mang", () => pm.expect(el.chats).to.be.an("array"));',
                   'pm.test("moi dong co type group|thread", () => pm.expect('
                   'el.chats.every(x => ["group","thread"].includes(x.type))).to.be.true);',
                   'pm.collectionVariables.set("chat_cursor", el.chatsPageInfo.nextCursor || "");')),
        req("Chi tiết đơn theo mã (card + Drive + thread)", "GET", "/api/client/v1/chat/cards/{{card_code}}",
            query=[("limit", "50")],
            desc="Chỉ cần **mã đơn** (`card.header.code`), không cần `messageId`. Trả `card` (đã lọc "
                 "`visibleTo`), `root`, `links` (các khối `{\"type\":\"links\"}` — vd Google Drive; `[]` = "
                 "CMS chưa gửi), `activity` (bot/hệ thống), `discussion` (người dùng), `threadCount`, "
                 "`pageInfo`.\n\n"
                 "- mã không tồn tại → `404 CARD_NOT_FOUND`\n"
                 "- không thuộc nhóm chứa card → `403 NOT_CONVERSATION_MEMBER`",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 / 403 (khong thuoc nhom) / 404 (chua co card)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 403, 404]));',
                   'if (pm.response.code === 200) {',
                   '  const e = pm.response.json().element;',
                   '  pm.test("co links, activity, discussion la mang", () => {',
                   '    pm.expect(e.links).to.be.an("array");',
                   '    pm.expect(e.activity).to.be.an("array");',
                   '    pm.expect(e.discussion).to.be.an("array"); });',
                   '}')),
        req("Bấm nút trên card", "POST", "/api/client/v1/chat/messages/{{card_message_id}}/actions",
            body={"action": "accept_task", "values": {"normItemId": "996802", "quantity": 2}},
            desc="Chat kiểm `visibleTo` trước (không được phép → `403 ACTION_NOT_ALLOWED`), rồi gửi "
                 "`card.action` sang CMS và áp card mới nếu `cardVersion` lớn hơn.\n\n"
                 "Chế độ fake (chưa bật CMS) hoặc tin nhắn không phải card → `400 cms_disabled` / "
                 "`400 message_has_no_card`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (CMS) hoac 400/403 (fake / khong co quyen)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
        req("Gửi form tạo yêu cầu", "POST", "/api/client/v1/chat/forms/create_task/submit",
            body={"values": CREATE_TASK_VALUES_SAMPLE,
                  "context": {"groupId": "{{group_id}}"}},
            desc="FE gửi `teamId`, `normItemId`, `quantity`, `size`, `urgencyCode`… trong `values`; "
                 "Chat **không diễn giải**, chuyển nguyên sang CMS trong event `form.submit` rồi trả "
                 "`toast`/`closeForm` cho UI. Gửi trong thread thì thêm `context.threadRootId`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (OK) / 400 (chua bat CMS) / 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
        req("Thông báo của tôi", "GET", "/api/client/v1/chat/notifications",
            query=[("unread", "true", True), ("limit", "20", True)],
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            desc="**Filter**: `unread=true` chỉ trả thông báo chưa đọc. "
                 "`element.unread` luôn là **tổng** số chưa đọc, không đổi theo filter (dùng cho badge).",
            tests=(OK,)),
        req("Thông báo chưa đọc", "GET", "/api/client/v1/chat/notifications",
            query=[("unread", "true")],
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            desc="Chỉ lấy thông báo chưa đọc. Phân trang cursor vẫn hoạt động cùng filter.",
            tests=(OK, 'pm.test("tat ca deu chua doc", () => pm.expect('
                   'pm.response.json().element.notifications.every(n => !n.read)).to.be.true);')),
        req("Đánh dấu đã đọc thông báo", "POST", "/api/client/v1/chat/notifications/read",
            body={"all": True},
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
    ],
)

cleanup = folder(
    "99 · Dọn dẹp (chạy cuối, tuỳ ý)",
    "Các request làm thay đổi/rút quyền — tách riêng để không phá flow ở trên. "
    "Chạy được ở mọi cấu hình.",
    [
        req("Rời nhóm / xoá thành viên", "DELETE",
            "/api/v1/conversations/{{group_id}}/members/{{user_id_2}}",
            desc="Tự rời nhóm được; xoá người khác cần owner/admin.",
            tests=('pm.test("200 hoặc 400/403", () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
    ],
)

DESCRIPTION = """\
# Chat Service — ABC Workspace

Chat (riêng/nhóm) + bot `@bot` gửi mẫu + tích hợp CMS (card, form, thông báo).
Go · Fiber (REST + WebSocket) · gRPC · PostgreSQL · GORM.

## Chuẩn bị (2 bước)

1. Chạy service: `make dev-up` (Docker, hot reload) hoặc `make up-deps && make migrate-up && make run`.
2. Lấy token dev và dán vào biến `token` của environment **Chat Service · Local**:
   `make token SUB=user-001 NAME="Nguyen Van A" ROLE=designer`
   Muốn thử phân công 2 người: tạo token thứ 2 (`SUB=user-002`), gọi `/me` bằng token đó,
   rồi dán `user_id` nhận được vào biến `user_id_2`.

## Cách chạy

**Mới dùng**: chạy folder `FLOW` — 16 bước đánh số theo đúng thứ tự thật, đặt biến `stop_at`
= số bước (vd `2.3`) để dừng lại xem kỹ.

**Tra cứu từng endpoint**: folder `00` → `09`. Mỗi request tự lưu id cần thiết
(`user_id`, `group_id`, `message_id`, `thread_id`, `actor_id`, `card_message_id`) vào collection
variable cho request sau dùng.

`group_id` nhận **cả UUID nội bộ lẫn `chatId` của CMS** (`wg_<hex>`) — một biến cho cả ba bề mặt
`/api/v1`, `/bot/v1`, `/api/client/v1`.

Response thành công: `{"data": ..., "meta": {"next_cursor","has_more"}}`.
Lỗi: `{"error":{"code","message","details"}}` — `code` ổn định, dùng để FE bắt lỗi.
Phân trang: cursor (`?cursor=<meta.next_cursor>&limit=30`), **không** dùng offset.

## Tích hợp CMS

Folder `08 · CMS → Chat (Bot API)` và `09 · API cho FE (hợp đồng CMS)` chỉ chạy khi service bật
`CMS_ENABLED=true`. Điền 2 biến environment: `service_key` (= `CHAT_SERVICE_KEY`) và `cms_token`
(access token do CMS phát). Header `X-Chat-Service-Key` + `Idempotency-Key` được gắn tự động cho
mọi request `/bot/v1`. Chi tiết: `docs/backend.md#tích-hợp-cms-designer-bot`.

## WebSocket & gRPC

Định dạng v2.1 của Postman không chứa được 2 loại request này, nên **mẫu nằm trong mô tả của
folder `10 · WebSocket & gRPC (mẫu frame)`**: mở folder → tab Documentation → copy khối JSON.

Trong đó có: 2 URL WebSocket và khác nhau ở đâu, 8 frame gửi lên kèm body đầy đủ, các frame
nhận về, bảng 13 trường hợp hay gặp, và 5 message mẫu cho gRPC.

Giao thức đầy đủ: `docs/frontend.md#giao-thức-websocket`. Danh sách RPC: `docs/backend.md#danh-sách-api`.
"""


# ---------------------------------------------------------------- FLOW
# Folder FLOW: chay tu tren xuong de hieu luong. Moi request co so thu tu;
# hai request cung cap thi 1.1, 1.2...

def stop(step):
    """Snippet dung runner tai buoc nay neu bien stop_at = so buoc.

    Dat stop_at = "2.2" roi Run collection => chay den 2.2 thi dung, xem ket qua
    roi doi stop_at sang buoc sau de di tiep. De trong = chay het.
    """
    return (
        f'if (String(pm.variables.get("stop_at") || "").trim() === "{step}") {{',
        f'  console.log("DUNG tai buoc {step} (stop_at). Doi stop_at de di tiep.");',
        '  postman.setNextRequest(null);',
        '}',
    )


def step(num, name, method, path, **kw):
    """Request trong FLOW: ten co so thu tu + tu dong gan diem dung."""
    kw["tests"] = tuple(kw.get("tests") or (OK,)) + stop(num)
    return req(f"{num} · {name}", method, path, **kw)


CMS_AUTH = {"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]}

flow = folder(
    "FLOW · Chạy từ trên xuống để hiểu luồng",
    "Toàn bộ vòng đời một nhóm chat và một đơn hàng, theo đúng thứ tự thật. **Chạy lần lượt từ 1.1**.\n\n"
    "### Dừng ở một bước\n"
    "Đặt biến `stop_at` = số bước (vd `3.2`) rồi **Run collection** → chạy tới bước đó thì dừng. "
    "Xem response, đổi `stop_at` sang bước sau để đi tiếp. Để trống = chạy hết.\n\n"
    "```bash\nmake postman-flow TOKEN=$(make -s token-cms CMS_USER=6) KEY=$CHAT_SERVICE_KEY [STOP=3.2]\n```\n\n"
    "### Đánh số\n"
    "`1.1`, `1.2`… = các bước **cùng cấp** trong một giai đoạn:\n\n"
    "| | Giai đoạn | Ai gọi |\n|---|---|---|\n"
    "| 1 | chuẩn bị | — |\n"
    "| 2 | nhóm & thành viên | FE + CMS |\n"
    "| 3 | tin nhắn | FE |\n"
    "| 4 | **vòng đời một đơn** | FE → CMS → Chat |\n"
    "| 5 | thông báo | CMS + FE |\n"
    "| 6 | dọn dẹp | — |\n\n"
    "### Điều quan trọng nhất về giai đoạn 4\n"
    "`POST /forms/create_task/submit` ký HMAC rồi chuyển sang CMS; CMS trả `ok` thì Chat chỉ ghi "
    "**một tin system** \"đã gửi yêu cầu\" vào nhóm. **Card đơn** chỉ xuất hiện khi **CMS gọi ngược** "
    "vào `POST /bot/v1/messages`.\n\n"
    "Ở local CMS không gọi tới được `localhost` của bạn, nên bước **4.3 đóng vai CMS** (dùng "
    "`service_key`) để bạn thấy đủ vòng. Trên môi trường thật, bước đó do CMS tự làm.\n\n"
    "### Cần gì trước\n"
    "- `token` + `cms_token` — `make token-cms CMS_USER=6` (người thật trong nhóm CMS)\n"
    "- `service_key` — `CHAT_SERVICE_KEY` trong `.env`, cho các bước `/bot/v1`\n\n"
    "`group_id` do bước 2.1 lưu, `card_message_id` do 4.3 lưu — không phải điền tay.",
    [
        # ---- 1 · chuan bi ----
        step("1.1", "Service còn sống?", "GET", "/healthz",
             desc="Chưa 200 thì không cần đi tiếp: `make dev-up` rồi `make dev-logs`.",
             tests=(OK, 'pm.test("status ok", () => pm.expect(pm.response.json().data.status).to.eql("ok"));')),
        step("1.2", "Tôi là ai (bề mặt FE)", "GET", "/api/client/v1/chat/auth/me",
             auth=CMS_AUTH,
             desc="Xác minh `cms_token` hợp lệ. 401 ở đây = token sai, hết hạn, hoặc `JWT_SECRET` "
                  "của app khác với lúc sinh token (bẫy hay gặp: `environment:` trong compose ghi "
                  "cứng `JWT_SECRET` sẽ đè `env_file`).",
             tests=(OK, 'const me = pm.response.json().element.me;',
                    'pm.test("co actorId", () => pm.expect(me).to.have.property("actorId"));',
                    'pm.collectionVariables.set("actor_id", me.actorId);',
                    'console.log("actor_id =", me.actorId, "| webTeams =",'
                    ' JSON.stringify((me.profile || {}).webTeams));')),

        # ---- 2 · nhom & thanh vien ----
        step("2.1", "Mở app: bootstrap", "GET", "/api/client/v1/chat/bootstrap",
             auth=CMS_AUTH,
             desc="Tất cả thứ FE cần khi mở app: `me`, `groups` (**chỉ nhóm mình là thành viên**), "
                  "`notifications`, `unreadNotifications`.\n\n"
                  "Bước này lưu `group_id` = nhóm đầu tiên, và `team_id_cms` = tổ đầu tiên của nhóm đó "
                  "(`groups[].cms.teams[].id`) — bước 4.1 cần đúng tổ **thuộc nhóm này**.",
             tests=(OK, 'const el = pm.response.json().element;',
                    'pm.test("co me + groups", () => pm.expect(el).to.include.all.keys('
                    '["me","groups","pinnedChats","chats","chatsPageInfo","notifications","unreadNotifications"]));',
                    'const g = (el.groups || [])[0];',
                    'if (g) {',
                    '  pm.collectionVariables.set("group_id", g.chatId || g.id);',
                    '  const t = ((g.cms || {}).teams || [])[0];',
                    '  if (t) pm.collectionVariables.set("team_id_cms", String(t.id));',
                    '  console.log("group_id =", g.chatId || g.id, "| teams =", JSON.stringify((g.cms||{}).teams));',
                    '}')),
        step("2.2", "Nhóm này có những ai?", "GET", "/bot/v1/groups/{{group_id}}/members",
             auth={"type": "noauth"},
             desc="**Điểm dừng hay dùng**: xác nhận thành viên trước khi gửi gì.\n\n"
                  "`id` trả về là **actorId** (`u_6`) — định danh Chat cấp cho CMS, không phải UUID. "
                  "Dùng giá trị này cho `visibleTo` của nút trên card và cho `notify[].actorId`.",
             tests=(OK, 'const items = pm.response.json().items || [];',
                    'pm.test("co thanh vien", () => pm.expect(items.length).to.be.above(0));',
                    'console.log("thanh vien =", items.map(i => i.id).join(", "));')),
        step("2.3", "CMS đẩy nhóm + thành viên", "PUT", "/bot/v1/groups/{{cms_chat_id}}",
             body={"name": "FLOW · nhóm từ CMS",
                   "members": [{"openId": "local:flow-1", "cmsUserId": 9001, "name": "Flow User",
                                "sources": ["team"], "teamCodes": ["DESIGN"]}]},
             auth={"type": "noauth"},
             desc="Chiều **CMS → Chat**. Tham số URL là `cms_chat_id` (dạng `wg_<hex>`), KHÔNG phải "
                  "`group_id` — vì nó chính là chatId mà CMS muốn tạo.\n\n"
                  "**Thay thế toàn bộ** danh sách thành viên: ai không có trong `members` mất quyền xem ngay.\n\n"
                  "Chat chỉ bắn event `reload` khi **thật sự** có thay đổi — gọi lại y nguyên payload "
                  "thì không ai nhận gì (chạy 2 lần sẽ thấy).\n\n"
                  "Bước này **không** đổi `group_id`: nhóm vừa đẩy chỉ có thành viên do CMS khai, "
                  "token của bạn không nằm trong đó nên các bước sau sẽ 403.",
             tests=('pm.test("200 (da bat CMS) / 404 (CMS_ENABLED=false)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',)),

        # ---- 3 · tin nhan ----
        step("3.1", "Gửi tin nhắn", "POST", "/api/client/v1/chat/groups/{{group_id}}/messages",
             body={"text": "FLOW: tin nhắn đầu tiên", "clientMsgId": "{{$guid}}"},
             auth=CMS_AUTH,
             desc="`clientMsgId` chống gửi trùng khi mạng chập chờn — gửi lại cùng giá trị thì "
                  "server trả tin cũ chứ không tạo thêm.\n\n"
                  "Chat đồng thời ghi event `message.created` vào **outbox** để gửi sang CMS (có retry), "
                  "khác với `form.submit` ở 4.1 là gọi đồng bộ.",
             tests=(OK, 'const el = pm.response.json().element;',
                    'pm.collectionVariables.set("message_id", el.message.id);')),
        step("3.2", "Trả lời trong thread", "POST", "/api/client/v1/chat/groups/{{group_id}}/messages",
             body={"text": "FLOW: trả lời", "replyToId": "{{message_id}}", "clientMsgId": "{{$guid}}"},
             auth=CMS_AUTH,
             desc="`replyToId` tạo thread. Tin gốc giữ nguyên, tin này có `threadRootId` = id tin gốc.",
             tests=(OK,)),
        step("3.3", "Đọc lịch sử", "GET", "/api/client/v1/chat/groups/{{group_id}}/messages",
             query=[("limit", "30")], auth=CMS_AUTH,
             desc="Mới nhất trước, phân trang bằng `pageInfo.nextCursor`. Card trong danh sách **đã "
                  "lọc nút theo `visibleTo`** của chính người gọi.\n\n"
                  "Chỉ có **tin gốc** — tin trả lời trong thread (`threadRootId`) không nằm ở đây.",
             tests=(OK, 'pm.test("khong co tin trong thread", () => pm.expect('
                        'pm.response.json().element.messages.every(m => !m.threadRootId)).to.be.true);')),
        step("3.4", "Đánh dấu đã đọc", "POST", "/api/client/v1/chat/groups/{{group_id}}/read",
             body={"messageId": "{{message_id}}"}, auth=CMS_AUTH,
             desc="Trả `unreadCount` mới. Realtime phát event `read` cho người khác trong nhóm.",
             tests=(OK,)),

        # ---- 4 · vong doi mot don ----
        step("4.1", "FE gửi form tạo đơn", "POST", "/api/client/v1/chat/forms/create_task/submit",
             body={"values": {**CREATE_TASK_VALUES_SAMPLE, "title": "FLOW · Banner Tết",
                              "teamId": "{{team_id_cms}}"},
                   "context": {"groupId": "{{group_id}}"}},
             auth=CMS_AUTH,
             desc="Chiều **FE → Chat → CMS**, gọi **đồng bộ** (chờ CMS trả lời) vì UI cần kết quả ngay.\n\n"
                  "Chat **không đọc `values`** — chỉ ký HMAC rồi chuyển nguyên sang CMS. Thêm trường mới "
                  "vào form thì không phải sửa Chat.\n\n"
                  "⚠️ **`statusCode: 200` và `ok: true` KHÔNG có nghĩa là tạo được đơn.** Chúng chỉ nói "
                  "\"Chat chuyển sang CMS được\" và \"CMS xử lý được request\". Kết quả nghiệp vụ nằm ở "
                  "`toast.type` và `closeForm`:\n\n"
                  "- `toast.type: \"success\"` + `closeForm: true` → đơn đã tạo\n"
                  "- `toast.type: \"error\"` + `closeForm: false` → CMS từ chối, giữ form để sửa\n\n"
                  "Lỗi hay gặp: `\"Chọn tổ thực hiện thuộc nhóm này\"` — `teamId` không thuộc nhóm. "
                  "Bước 2.1 đã lấy đúng tổ vào `team_id_cms`; nếu nhóm không có tổ nào thì biến rỗng "
                  "và bước này sẽ bị từ chối.\n\n"
                  "**Bước này chỉ ghi một tin system** (`metadata.kind = \"form_submitted\"`) vào nhóm khi CMS "
                  "trả `ok`. Card đơn đến từ 4.3.",
             tests=('pm.test("200 / 400 (chua bat CMS) / 403 (khong thuoc nhom)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',
                    'if (pm.response.code === 200) {',
                    '  const el = pm.response.json().element;',
                    '  console.log("toast =", JSON.stringify(el.toast), "| closeForm =", el.closeForm);',
                    '  pm.test("doc toast, khong chi doc ok", () => pm.expect(el).to.have.property("closeForm"));',
                    '}')),
        step("4.2", "Chat đã gửi event sang CMS chưa?", "GET", "/api/client/v1/chat/notifications",
             query=[("limit", "1")], auth=CMS_AUTH,
             desc="Bước kiểm tạm (API này chỉ để request không rỗng).\n\n"
                  "Thứ cần xem là **outbox** — chạy lệnh này ở terminal:\n\n"
                  "```bash\nmake dev-psql\n"
                  "SELECT event_type, status, attempts, last_error FROM webhook_outbox\n"
                  "  ORDER BY created_at DESC LIMIT 5;\n```\n\n"
                  "`status=sent` là CMS đã nhận. `form.submit` **không** qua outbox (gọi đồng bộ), "
                  "chỉ `message.created` đi qua đây.",
             tests=(OK,)),
        step("4.3", "CMS đẩy card đơn vào nhóm", "POST", "/bot/v1/messages",
             body={"groupId": "{{group_id}}",
                   "bot": {"id": 1, "name": "Designer Bot"},
                   "card": {"schema": "abc.card/v1",
                            "header": {"code": "WEB-20261118-0006", "title": "FLOW · Banner Tết",
                                       "status": {"text": "Chưa nhận", "tone": "warning"}},
                            "elements": [{"type": "field", "label": "Số lượng", "value": "3"},
                                         {"type": "button", "text": "Nhận việc",
                                          "action": "accept_task", "visibleTo": ["{{actor_id}}"]}]}},
             auth={"type": "noauth"},
             desc="**Đây là bước làm card hiện ra trong nhóm** — chiều CMS → Chat.\n\n"
                  "Trên môi trường thật, CMS tự gọi bước này ngay sau khi tạo đơn ở 4.1. Ở local CMS "
                  "không gọi tới được `localhost` của bạn, nên request này **đóng vai CMS** bằng "
                  "`service_key`. Đó là lý do submit ở 4.1 thành công mà nhóm vẫn trống.\n\n"
                  "⚠️ Khoá của nút là **`action`**, KHÔNG phải `actionId` — sai tên thì card vẫn tạo "
                  "được nhưng bấm nút trả `403 action_not_allowed`.\n\n"
                  "`visibleTo` = danh sách `actorId` được thấy nút đó. Chat lọc nút theo **từng người xem** "
                  "trước khi gửi xuống UI, mỗi người nhận một bản card riêng kể cả qua realtime.",
             tests=('pm.test("200 / 404 (chua bat CMS)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',
                    'if (pm.response.code === 200) {',
                    '  const d = pm.response.json();',
                    '  pm.collectionVariables.set("card_message_id", d.messageId);',
                    '  pm.test("co cardVersion", () => pm.expect(d).to.have.property("cardVersion"));',
                    '}')),
        step("4.4", "FE bấm nút trên card", "POST",
             "/api/client/v1/chat/messages/{{card_message_id}}/actions",
             body={"action": "accept_task", "values": {"normItemId": "996802", "quantity": 2}},
             auth=CMS_AUTH,
             desc="Chat kiểm **trước khi gọi CMS**: action có trong card? người bấm có trong `visibleTo`? "
                  "Không thì `403 action_not_allowed` và **không** gọi CMS.\n\n"
                  "Qua được thì Chat gửi webhook `card.action` sang CMS; CMS trả card mới, Chat thay card "
                  "theo `cardVersion` rồi phát realtime cho cả nhóm.",
             tests=('pm.test("200 / 403 (khong trong visibleTo) / 400",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
        step("4.5", "CMS cập nhật đơn — gộp 3 việc 1 request", "PATCH",
             "/bot/v1/messages/{{card_message_id}}",
             body={"card": {"schema": "abc.card/v1",
                            "header": {"code": "WEB-20261118-0006", "title": "FLOW · Banner Tết",
                                       "status": {"text": "Đã nộp bài", "tone": "success"}},
                            "elements": [{"type": "text", "text": "Chờ duyệt"}]},
                   "cardVersion": 2,
                   "thread": {"text": "Flow User đã nộp bài · Chờ duyệt",
                              "bot": {"id": 1, "name": "Designer Bot"}},
                   "notify": [{"actorId": "{{actor_id}}",
                               "text": "Đơn WEB-20261118-0006 đã nộp bài"}]},
             auth={"type": "noauth"},
             desc="**Mỗi lần đơn đổi trạng thái thì dùng request này** — thay cho 3 request riêng "
                  "(`PATCH` card + `POST .../thread` + `POST /notifications`). Một `Idempotency-Key`, "
                  "một chỗ xử lý lỗi.\n\n"
                  "`thread` và `notify` **tuỳ chọn**; không gửi thì hành vi y như trước.\n\n"
                  "**Quy ước đã kiểm chứng:**\n"
                  "- `cardVersion` không lớn hơn bản đang lưu → `{ok, ignored:true}` và **bỏ qua cả** "
                  "`thread`/`notify` (đang gửi trạng thái cũ thì không được ghi thêm dòng diễn biến cũ)\n"
                  "- `thread`/`notify` lỗi → vẫn `200`, card đã đổi xong, lỗi báo theo từng phần: "
                  "`{\"thread\":{\"error\":{\"code\":\"text_required\"}}}`\n\n"
                  "Chạy lại request này lần nữa sẽ thấy `ignored: true` — vì `cardVersion` vẫn là 2.",
             tests=('pm.test("200 / 404", () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',
                    'if (pm.response.code === 200) {',
                    '  const d = pm.response.json();',
                    '  console.log("ignored =", d.ignored, "| thread =", JSON.stringify(d.thread));',
                    '  pm.test("ignored thi khong co thread/notify", () => {',
                    '    if (d.ignored) { pm.expect(d).to.not.have.property("thread");',
                    '                     pm.expect(d).to.not.have.property("notify"); }',
                    '  });',
                    '}')),

        # ---- 5 · thong bao ----
        step("5.1", "Thông báo chưa đọc của tôi", "GET", "/api/client/v1/chat/notifications",
             query=[("unread", "true")], auth=CMS_AUTH,
             desc="Bước 4.5 vừa tạo một thông báo qua field `notify` — nó phải xuất hiện ở đây.\n\n"
                  "`element.unread` là **tổng** số chưa đọc (cho badge), không đổi theo filter.",
             tests=(OK, 'const el = pm.response.json().element;',
                    'console.log("so thong bao chua doc =", el.unread);')),
        step("5.2", "Đánh dấu đã đọc hết", "POST", "/api/client/v1/chat/notifications/read",
             body={"all": True}, auth=CMS_AUTH,
             desc="Trả số dòng vừa đánh dấu. `{\"ids\": [...]}` để đánh dấu từng cái.",
             tests=(OK,)),

        # ---- 6 · don dep ----
        step("6.1", "Xoá tin nhắn đã gửi ở 3.1", "DELETE", "/api/v1/messages/{{message_id}}",
             desc="Xoá mềm: tin còn trong DB nhưng FE không hiện, và phát event `message.deleted`. "
                  "Bỏ qua nếu muốn giữ để xem tiếp.\n\n"
                  "Card ở 4.3 thì để lại — xoá nhóm/dữ liệu test bằng `make dev-down-v` nếu cần sạch hẳn.",
             tests=('pm.test("200 / 403 / 404", () => pm.expect(pm.response.code)'
                    '.to.be.oneOf([200, 403, 404]));',)),
    ],
)


# ---------------------------------------------------------------- WebSocket / gRPC
# v2.1 JSON khong co kieu request WebSocket/gRPC. Giu mau o day duoi dang tai
# lieu: mo folder trong Postman -> tab Documentation -> copy khoi JSON can dung.

WS_DOC = """\
Postman khong import duoc WebSocket tu file v2.1, nen mo tay:
**New -> WebSocket**, dan URL, **Connect**, roi dan tung khoi JSON ben duoi vao o Message.

## 1. Hai URL

```
ws://localhost:8080/api/client/v1/chat/ws?token={{cms_token}}    <- FE dung cai nay
ws://localhost:8080/ws?token={{token}}                           <- cong cu noi bo
```

Cung mot hub, cung event. Khac o **ten field**: duong FE tra camelCase trung ten voi REST
(`text`, `createdAt`, `threadRootId`), duong noi bo tra snake_case + nguyen struct domain.

Token di qua `?token=` vi browser khong set duoc header khi mo WebSocket. Thu tu uu tien:
`?token=` -> `Sec-WebSocket-Protocol` -> header `Authorization`.

Khong co token rieng cho WebSocket: **dung lai dung access token dang goi REST**.

## 2. Frame gui len (client -> server)

Moi frame la 1 JSON object. `id` do client tu dat, server tra lai trong `ref` cua `ack`/`error`
de biet ack cua frame nao.

`conversationId` nhan **ca UUID lan chatId cua CMS** (`wg_...`). Khoa chap nhan ca camelCase
lan snake_case (`conversationId` = `conversation_id`, `text` = `content`).

### 2.1 ping - giu ket noi, do RTT

```json
{"id":"p1","type":"ping"}
```
-> `{"type":"pong","ref":"p1","payload":{"server_now":"..."}}`

### 2.2 message.send - gui tin nhan

```json
{
  "id": "c1",
  "type": "message.send",
  "payload": {
    "conversationId": "{{group_id}}",
    "type": "text",
    "text": "Tin nhan gui qua WebSocket",
    "clientMsgId": "ws-1"
  }
}
```

### 2.2b message.send - nhac ten (@)

```json
{
  "id": "c1m",
  "type": "message.send",
  "payload": {
    "conversationId": "{{group_id}}",
    "text": "Nho @Tran Designer xem giup",
    "mentions": ["{{actor_id}}"],
    "mentionAll": false,
    "clientMsgId": "ws-m1"
  }
}
```

`mentions` = **actorId** (`u_<cmsUserId>`) cua nguoi duoc nhac, KHONG phai ho ten. Lay tu
`bootstrap` -> `groups[].members[].actorId`. User chua co actorId (token dev) thi dung `userId` (UUID).
Server chi giu thanh vien nhom, bo chinh minh, bo trung, toi da 100. Nguoi ngoai nhom bi bo qua
(khong bao loi).

Duong noi bo `/ws` dung snake_case: `"mentions": [...]`, `"mention_all": false`.

Ket qua:
- nguoi gui nhan `ack` (ref = `c1m`);
- ca nhom nhan frame `message` co `mentions`, `mentionAll`, `mentionedMe` (tinh theo tung nguoi nhan);
- MOI nguoi duoc nhac nhan them frame `notification` (muc 3.6).

### 2.2c message.send - nhac ca nhom (@all)

```json
{
  "id": "c1a",
  "type": "message.send",
  "payload": {
    "conversationId": "{{group_id}}",
    "text": "@all Chieu nay hop 15h nhe",
    "mentionAll": true,
    "clientMsgId": "ws-m2"
  }
}
```

Chi admin / tro ly cua nhom (`bootstrap` -> `groups[].canMentionAll = true`). `mentions` cua tin =
moi thanh vien tru nguoi gui. Nguoi khac gui:

```json
{"type":"error","ref":"c1a","payload":{"code":"mention_all_forbidden",
  "message":"Chỉ admin / trợ lý được nhắc @all"}}
```
Tin **khong** duoc tao.

### 2.3 message.send - tra loi trong thread

```json
{
  "id": "c2",
  "type": "message.send",
  "payload": {
    "conversationId": "{{group_id}}",
    "text": "Phan hoi trong thread",
    "threadRootId": "{{message_id}}",
    "clientMsgId": "ws-2"
  }
}
```

### 2.4 message.send - kem anh/tep

```json
{
  "id": "c3",
  "type": "message.send",
  "payload": {
    "conversationId": "{{group_id}}",
    "type": "image",
    "text": "Anh mau",
    "attachments": [{
      "kind": "image",
      "url": "https://cdn.example.com/a.png",
      "name": "a.png",
      "mime_type": "image/png",
      "size_bytes": 20480,
      "width": 800,
      "height": 600
    }],
    "clientMsgId": "ws-3"
  }
}
```

### 2.5 message.read - danh dau da doc

```json
{
  "id": "c4",
  "type": "message.read",
  "payload": {"conversationId": "{{group_id}}", "messageId": "{{message_id}}"}
}
```

### 2.6 presence.list - ai dang online

```json
{"id":"p1","type":"presence.list"}
```

Khong truyen gi = moi nguoi dung chung it nhat 1 nhom voi minh. Gioi han trong 1 nhom:

```json
{"id":"p2","type":"presence.list","payload":{"groupId":"{{group_id}}"}}
```

Tra ve `ack` voi `{count, items:[{userId, actorId, name, presence, online, reason?, lastSeenAt?}]}`.
`presence`: `online` | `offline` | `off_duty`.

Event `presence` chi ban KHI DOI, nen frame nay dung luc moi mo tab de lay anh chup hien tai.
Doc tu bang `users` nen chay nhieu pod van dung.

### 2.7 typing - dang nhap

```json
{
  "id": "c5",
  "type": "typing",
  "payload": {"conversationId": "{{group_id}}", "typing": true}
}
```
Khong luu DB, **khong co ack**. Chi nhung nguoi khac trong nhom nhan event `typing`.

### 2.8 message.delete - xoa mem

```json
{"id":"c6","type":"message.delete","payload":{"messageId":"{{message_id}}"}}
```

### 2.9 template.send - gui mau cho bot

```json
{
  "id": "c7",
  "type": "template.send",
  "payload": {
    "conversationId": "{{group_id}}",
    "bot_slug": "bot",
    "template_code": "yeu_cau",
    "payload": {"tieu_de": "Banner Tet", "to": "design", "so_luong": 2, "dinh_muc": "A4"},
    "clientMsgId": "ws-4"
  }
}
```

## 3. Frame nhan ve (server -> client)

### 3.1 ready - ngay sau khi ket noi

Duong FE:
```json
{"type":"ready","payload":{
  "userId":"6a63be69-...","actorId":"u_6","name":"tester",
  "role":"designer","connId":"81f68ee6-...","serverNow":"2026-11-20T10:00:00Z"}}
```
Duong noi bo: `conn_id`, `display_name`, `external_id`, `server_now`.

**Nhin frame nay truoc tien**: no cho biet tab dang dang nhap bang AI. Dan sai token
hoac tool tu gan header `Authorization` cua nguoi khac la thay ngay o day.

### 3.2 ack - rieng cho nguoi vua gui

```json
{"type":"ack","ref":"c1","payload":{
  "client_msg_id":"ws-1",
  "message":{"id":"...","conversationId":"...","text":"...","createdAt":"..."},
  "bot_reply":null,"submission":null,"duplicated":false}}
```
`duplicated: true` = `clientMsgId` da gui roi, server **khong** tao tin moi (chong gui trung
khi mang chap chon).

### 3.3 message - tin moi hoac tin duoc cap nhat

```json
{"type":"message","eventId":"...","conversationId":"...","ts":"...","payload":{
  "id":"...","conversationId":"...","type":"text","senderType":"user",
  "text":"...","createdAt":"...","clientMsgId":"...",
  "senderId":"...","sender":{"actorId":"u_6","name":"tester","cmsUserId":6,
                             "openId":"...","presence":"online","role":"designer"},
  "card":{...},"cardVersion":2,"actions":["accept_task"],
  "mentions":["u_55"],"mentionAll":false,"mentionedMe":true}}
```
`mentions` / `mentionAll` / `mentionedMe` chi co khi tin co nhac ten. `mentionedMe = true` ->
to vien noi bat tin. Card cua CMS cung co the co `mentions` (nguoi lam, TL, Tro ly).
FE **upsert theo `id`**: card doi cung den bang `type: "message"` chu khong co type rieng.
`card` da loc nut theo `visibleTo` cua chinh nguoi nhan, `actions` la cac action nguoi do
duoc bam.

### 3.4 Cac event con lai

| `type` | Khi nao | Payload chinh |
|---|---|---|
| `message.deleted` | ai do xoa tin | `conversationId`, `messageId` |
| `read` | ai do danh dau da doc | `conversationId`, `userId`, `messageId`, `seenAt` |
| `typing` | ai do dang nhap | `conversationId`, `userId`, `typing` |
| `presence` | nguoi chung nhom doi trang thai | `actorId`, `userId`, `presence`, `online`, `reason` |
| `notification` | CMS tao thong bao cho minh, HOAC minh bi nhac @ (muc 3.6) | `id`, `text`, `read`, `createdAt`, `messageId`, `conversationId`, `groupId`, `threadRootId`, `payload` |
| `user` | ho so nguoi chung nhom doi tren CMS (ho ten, username, avatar, vai tro) | `userId`, `actorId`, `name`, `username`, `avatarUrl`, `role`, `status` — cap nhat ten tren chip @ |
| `reload` | nhom doi -> tai lai danh sach | `reason` (`group.members_changed` khi them/bot thanh vien, `group.updated` khi doi ten/cms_info), `conversationId`, `chatId`. **Khong doi gi thi khong ban** — khong phai moi 60s mot lan |

### 3.6 notification - bi nhac ten (@)

Chi gui cho nguoi duoc nhac (khong gui cho nguoi gui):

```json
{"type":"notification","payload":{
  "id":"...","text":"An nhắc đến bạn trong Nhóm Web: Nho @Tran Designer xem giup",
  "read":false,"createdAt":"...",
  "messageId":"<id tin>","conversationId":"...","groupId":"wg_...",
  "threadRootId":"<chi khi tin nam trong thread>",
  "payload":{"kind":"mention","mentionAll":false}}}
```
`payload.kind = "mention"` de phan biet voi thong bao cua CMS. Bam vao: mo `groupId`, co
`threadRootId` thi mo thread do, roi cuon toi `messageId`. Nguoi dang offline van thay thong bao
nay qua `bootstrap` / `GET /chat/notifications`.

### 3.5 error - frame bi tu choi

```json
{"type":"error","ref":"c1","payload":{"code":"not_conversation_member",
  "message":"Ban khong thuoc cuoc hoi thoai nay"}}
```

## 4. Cac truong hop hay gap

| Tinh huong | Ket qua |
|---|---|
| Thieu `token` | HTTP **401** luc handshake, khong len duoc WebSocket |
| Token sai / het han | HTTP **401** luc handshake |
| Khong phai thanh vien nhom | ket noi OK, nhung frame bi tra `error` `not_conversation_member` |
| `conversationId` khong ton tai | `error` `conversation_not_found` |
| `conversationId` de trong | `error` `invalid_request` |
| JSON sai cu phap / payload sai kieu | `error` `invalid_payload` |
| `type` khong biet | `error` `unknown_frame_type` |
| Gui lai cung `clientMsgId` | `ack` voi `duplicated: true`, khong tao tin moi |
| `text` rong va khong co attachment | `error` `empty_message` |
| `mentionAll: true` ma khong phai admin / tro ly | `error` `mention_all_forbidden`, tin khong duoc tao |
| `mentions` chua nguoi ngoai nhom / chinh minh | bo qua nguoi do, tin van gui binh thuong |
| Gui `typing` | khong co ack — dung cho la loi |
| Khong hoat dong lau | server tu gui WebSocket **ping** moi `WS_PING_INTERVAL` (25s); client khong tra pong trong `WS_READ_TIMEOUT` (90s) thi bi dong |
| Mat ket noi | Pub/Sub la at-most-once: **goi lai REST** `GET /groups/:id/messages?cursor=` de dong bo, dung trong cho event bu |

## 5. gRPC

Postman: **New -> gRPC**, URL `{{grpc_url}}` (`localhost:9090`), metadata
`authorization: Bearer {{token}}`. Reflection dang bat o local nen Postman tu tai duoc
danh sach method (`GRPC_REFLECTION=false` o prod).

| Method | Message mau |
|---|---|
| `chat.v1.ChatService/ListConversations` | `{"page": {"limit": 10}}` |
| `chat.v1.ChatService/ListMessages` | `{"conversation_id": "{{group_id}}", "page": {"limit": 20}}` |
| `chat.v1.ChatService/SendMessage` | `{"conversation_id": "{{group_id}}", "content": "Tin qua gRPC", "client_msg_id": "g-1"}` |
| `chat.v1.BotService/ListBotTemplates` | `{"bot_slug": "bot"}` |
| `chat.v1.BotService/SubmitBotTemplate` | `{"bot_slug": "bot", "template_code": "yeu_cau", "conversation_id": "{{group_id}}", "payload_json": "{\\"tieu_de\\":\\"Yeu cau tu gRPC\\",\\"to\\":\\"design\\",\\"so_luong\\":2,\\"dinh_muc\\":\\"A4\\"}"}` |

`grpcurl -plaintext localhost:9090 list` de xem toan bo service.
"""

websocket = folder("10 · WebSocket & gRPC (mẫu frame)", WS_DOC, [
    req("Mở trang demo để thử WebSocket thật", "GET", "/demo/live.html",
        desc="Giao diện chat thật, tự nối `/api/client/v1/chat/ws` bằng token bạn dán vào. "
             "Dùng khi muốn thấy event chạy mà không phải dán frame bằng tay.\\n\\n"
             "Cần `WEB_DEMO_DIR` khác rỗng (mặc định `./web`; prod để rỗng nên sẽ 404).",
        auth={"type": "noauth"},
        tests=('pm.test("200 (co WEB_DEMO_DIR) / 404 (da tat)",'
               ' () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',)),
])

collection = {
    "info": {
        "_postman_id": COLLECTION_ID,
        "name": "Chat Service · A Workspace [GRPC]",
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
                    "// group_id = id nhóm chat, dùng cho CẢ BA bề mặt (/api/v1, /bot/v1,",
                    "// /api/client/v1): UUID nội bộ hoặc chatId của CMS (wg_<hex>) đều được.",
                    "// Folder 01 (hoặc FLOW) tự lưu biến này sau khi tạo/đồng bộ nhóm.",
                    "// Ngoại lệ duy nhất: PUT /bot/v1/groups/:id dùng cms_chat_id, vì tham số",
                    "// đó LÀ chatId mà CMS muốn tạo — truyền UUID vào sẽ đẻ ra nhóm lạ.",
                    "// Chưa chạy folder CMS thì dùng tạm tin nhắn thường để các request không vỡ.",
                    "if (!pm.variables.get('card_message_id') && pm.variables.get('message_id')) {",
                    "  pm.collectionVariables.set('card_message_id', pm.variables.get('message_id'));",
                    "}",
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
        {"key": "docs_key", "value": "", "type": "string"},
        {"key": "cms_token", "value": "", "type": "string"},
        {"key": "group_id", "value": "", "type": "string"},
        {"key": "cms_chat_id", "value": "wg_postman_demo", "type": "string"},
        {"key": "stop_at", "value": "", "type": "string"},
        # Tổ của CMS, do bước FLOW 2.1 lấy từ groups[].cms.teams[].id.
        {"key": "team_id_cms", "value": "", "type": "string"},
        # Ten action tren card, do FLOW [FE] buoc 3.1 lay tu actions[0].
        {"key": "card_action", "value": "", "type": "string"},
        {"key": "actor_id", "value": "", "type": "string"},
        {"key": "card_message_id", "value": "", "type": "string"},
        # Cursor trang thanh ben (chats) tiep theo, do request Bootstrap luu (lazy load).
        {"key": "chat_cursor", "value": "", "type": "string"},
        # Ma don tren card (header.code) — trung voi card mau cua request "Gửi card vào nhóm".
        {"key": "card_code", "value": "WEB-20261118-0006", "type": "string"},
        {"key": "user_id", "value": "", "type": "string"},
        {"key": "user_id_2", "value": "", "type": "string"},
        {"key": "direct_conversation_id", "value": "", "type": "string"},
        {"key": "message_id", "value": "", "type": "string"},
        {"key": "thread_id", "value": "", "type": "string"},
        {"key": "reply_message_id", "value": "", "type": "string"},
        {"key": "attachment_id", "value": "", "type": "string"},
    ],
    "item": [flow, start, conversations, messages, bots, errors,
             cms_bot, cms_client, websocket, cleanup],
}

environment = {
    "id": ENV_ID,
    "name": "Chat Service · Local",
    "values": [
        {"key": "base_url", "value": "http://localhost:8080", "type": "default", "enabled": True},
        {"key": "grpc_url", "value": "localhost:9090", "type": "default", "enabled": True},
        {"key": "ws_url", "value": "ws://localhost:8080/ws", "type": "default", "enabled": True},
        # WebSocket tren be mat FE (hop dong CMS) — dung chung cms_token voi REST.
        {"key": "ws_client_url", "value": "ws://localhost:8080/api/client/v1/chat/ws",
         "type": "default", "enabled": True},
        {"key": "token", "value": "", "type": "secret", "enabled": True},
        {"key": "token_2", "value": "", "type": "secret", "enabled": True},
        {"key": "cms_token", "value": "", "type": "secret", "enabled": True},
        {"key": "service_key", "value": "", "type": "secret", "enabled": True},
        {"key": "docs_key", "value": "", "type": "secret", "enabled": True},
        {"key": "user_id_2", "value": "", "type": "default", "enabled": True},
        # CO Y khong khai group_id o environment: bien environment LAN AT bien
        # collection, nen mot `group_id` rong o day se vo hieu hoa gia tri ma
        # folder FLOW / 01 vua luu => URL thanh /conversations//messages -> 404.
        # Can ghim mot nhom cu the thi truyen --env-var group_id=... khi chay CLI.
        #
        # stop_at: dat = so buoc (vd 2.3) de folder FLOW dung lai o do.
        {"key": "stop_at", "value": "", "type": "default", "enabled": True},
    ],
    "_postman_variable_scope": "environment",
}


# ================================================================ BAN TACH
# Hai collection rieng de gui cho tung doi. Dung lai chinh cac folder o tren,
# chi them FLOW viet theo goc nhin cua ben do.

flow_fe = folder(
    "FLOW · Luồng của FE",
    "Thứ tự FE gọi, từ lúc mở app tới lúc bấm nút trên card. **Chạy lần lượt từ 1.1**.\n\n"
    "### Chuẩn bị\n"
    "Điền **`cms_token`** = access token do CMS phát (`POST /api/client/v1/workspace/auth/login` "
    "của CMS). Chat **không có login** và không phát token riêng.\n\n"
    "Local test: `make token-cms CMS_USER=6` sinh token dev đóng vai một user CMS đã đồng bộ.\n\n"
    "### Dừng ở một bước\n"
    "Đặt `stop_at` = số bước (vd `3.2`) rồi Run collection.\n\n"
    "### Realtime\n"
    "Mở WebSocket **một lần** khi vào app và giữ suốt phiên:\n\n"
    "```\nws://localhost:8080/api/client/v1/chat/ws?token=<cms_token>\n```\n\n"
    "Cùng access token với REST, field trả về camelCase **trùng tên** với REST. Frame mẫu + "
    "13 trường hợp hay gặp: folder `WebSocket (mẫu frame)`.",
    [
        step("1.1", "Service còn sống?", "GET", "/healthz",
             auth={"type": "noauth"},
             desc="Chưa 200 thì không cần đi tiếp.",
             tests=(OK,)),
        step("1.2", "Tôi là ai", "GET", "/api/client/v1/chat/auth/me",
             auth=CMS_AUTH,
             desc="Xác minh token và lấy hồ sơ. Lưu `actor_id` — dùng để biết nút nào trên card "
                  "là của mình.\n\n401 ở đây = token sai hoặc hết hạn → gọi CMS "
                  "`POST /workspace/auth/refresh` rồi thử lại.",
             tests=(OK, 'const me = pm.response.json().element.me;',
                    'pm.test("co actorId", () => pm.expect(me).to.have.property("actorId"));',
                    'pm.collectionVariables.set("actor_id", me.actorId);')),
        step("2.1", "Mở app: bootstrap", "GET", "/api/client/v1/chat/bootstrap",
             auth=CMS_AUTH,
             desc="**Một request là đủ để vẽ xong sidebar**: `me`, `groups` (chỉ nhóm mình là "
                  "thành viên, kèm `unreadCount` + `members`), `notifications`, "
                  "`unreadNotifications`.\n\n"
                  "`groups[].cms.teams` là danh sách tổ của nhóm — dùng để dựng dropdown ở bước 4.1, "
                  "**đừng hardcode**.\n\n"
                  "Ngay sau bước này: mở WebSocket (xem mô tả folder).",
             tests=(OK, 'const el = pm.response.json().element;',
                    'pm.test("co du khoa", () => pm.expect(el).to.include.all.keys('
                    '["me","groups","pinnedChats","chats","chatsPageInfo","notifications","unreadNotifications"]));',
                    'const g = (el.groups || [])[0];',
                    'if (g) {',
                    '  pm.collectionVariables.set("group_id", g.chatId || g.id);',
                    '  const t = ((g.cms || {}).teams || [])[0];',
                    '  if (t) pm.collectionVariables.set("team_id_cms", String(t.id));',
                    '}')),
        step("3.1", "Mở một nhóm: đọc lịch sử", "GET",
             "/api/client/v1/chat/groups/{{group_id}}/messages",
             query=[("limit", "30")], auth=CMS_AUTH,
             desc="Mới nhất trước. Cuộn lên: thêm `cursor=<pageInfo.nextCursor>`.\n\n"
                  "Card trong danh sách **đã lọc nút theo `visibleTo`** của chính bạn, và `actions` "
                  "liệt kê action bạn được bấm — cứ render hết những gì nhận được.\n\n"
                  "`{{group_id}}` nhận cả `wg_…` (chatId của CMS) lẫn UUID nhóm.",
             tests=(OK, 'const el = pm.response.json().element;',
                    'const m = (el.messages || [])[0];',
                    'if (m) pm.collectionVariables.set("message_id", m.id);',
                    '// Uu tien card mA NGUOI NAY duoc bam (actions khong rong) de buoc 4.2 chay that;',
                    '// khong co thi lay card bat ky — buoc 4.2 se tra 403, van la ket qua dung.',
                    'const msgs = el.messages || [];',
                    'const card = msgs.find(x => x.card && (x.actions || []).length) || msgs.find(x => x.card);',
                    'if (card) {',
                    '  pm.collectionVariables.set("card_message_id", card.id);',
                    '  // Lay LUON ten action dau tien de buoc 4.2 bam dung nut co that.',
                    '  const a = (card.actions || [])[0];',
                    '  if (a) pm.collectionVariables.set("card_action", a);',
                    '  console.log("card =", card.id, "| actions =", JSON.stringify(card.actions || []));',
                    '}')),
        step("3.2", "Gửi tin nhắn", "POST",
             "/api/client/v1/chat/groups/{{group_id}}/messages",
             body={"text": "Xin chào cả nhóm", "clientMsgId": "{{$guid}}"},
             auth=CMS_AUTH,
             desc="`clientMsgId` do FE sinh (UUID) — gửi lại cùng giá trị thì server trả tin cũ "
                  "chứ không tạo thêm. Đây là cách chống trùng khi mạng chập chờn.\n\n"
                  "Gửi qua WebSocket (`message.send`) cũng được, cùng kết quả; dùng REST khi WS đứt.",
             tests=(OK, 'pm.collectionVariables.set("message_id",'
                    ' pm.response.json().element.message.id);')),
        step("3.3", "Trả lời trong thread", "POST",
             "/api/client/v1/chat/groups/{{group_id}}/messages",
             body={"text": "Trả lời trong thread", "replyToId": "{{message_id}}",
                   "clientMsgId": "{{$guid}}"},
             auth=CMS_AUTH,
             desc="`replyToId` = tin đang trả lời. Tin mới có `threadRootId` = id tin gốc; tin gốc "
                  "giữ nguyên nhưng `replyCount`/`threadCount` tăng → vẽ badge.\n\n"
                  "Mở cả thread: `GET /api/client/v1/chat/messages/{id}/replies?mode=thread`.",
             tests=(OK,)),
        step("3.4", "Đánh dấu đã đọc", "POST",
             "/api/client/v1/chat/groups/{{group_id}}/read",
             body={"messageId": "{{message_id}}"}, auth=CMS_AUTH,
             desc="Gọi khi người dùng xem tới tin mới nhất. Trả `unreadCount` mới; người khác "
                  "trong nhóm nhận event realtime `read` để cập nhật dấu đã xem.",
             tests=(OK,)),
        step("4.1", "Gửi form tạo đơn", "POST",
             "/api/client/v1/chat/forms/create_task/submit",
             body={"values": {**CREATE_TASK_VALUES_SAMPLE, "teamId": "{{team_id_cms}}"},
                   "context": {"groupId": "{{group_id}}"}},
             auth=CMS_AUTH,
             desc="`values` do **CMS** định nghĩa (spec mục 6.2), Chat chuyển nguyên không đọc. "
                  "`teamId` phải là tổ **thuộc nhóm này** — lấy từ `groups[].cms.teams[].id` ở bước 2.1.\n\n"
                  "⚠️ **`statusCode: 200` và `ok: true` KHÔNG có nghĩa là tạo được đơn.** Kết quả "
                  "nghiệp vụ nằm ở `toast.type` và `closeForm`:\n\n"
                  "```js\nif (el.toast) toast(el.toast.text, el.toast.type);\n"
                  "if (el.closeForm) closeForm();   // false = chưa xong, giữ form để sửa\n```\n\n"
                  "Gửi tổ không thuộc nhóm → `toast: {type:\"error\", text:\"Chọn tổ thực hiện "
                  "thuộc nhóm này\"}`, `closeForm: false`.\n\n"
                  "**Card đơn không đến từ response này.** Nó xuất hiện qua realtime khi CMS đẩy "
                  "card vào nhóm — FE chỉ cần nghe event `message` và upsert theo `id`.",
             tests=('pm.test("200 / 400 (chua bat CMS) / 403 (khong thuoc nhom)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',
                    'if (pm.response.code === 200) {',
                    '  const el = pm.response.json().element;',
                    '  console.log("toast =", JSON.stringify(el.toast), "| closeForm =", el.closeForm);',
                    '}')),
        step("4.2", "Bấm nút trên card", "POST",
             "/api/client/v1/chat/messages/{{card_message_id}}/actions",
             body={"action": "{{card_action}}", "values": {"normItemId": "996802", "quantity": 2}},
             auth=CMS_AUTH,
             desc="`action` lấy từ `actions[]` của card — **chỉ hiện nút nào có trong đó**. "
                  "Bước 3.1 đã tự điền `card_action` = action đầu tiên bạn được bấm.\n\n"
                  "Người không được phép bấm → `403 ACTION_NOT_ALLOWED` (server chặn, không phụ "
                  "thuộc UI).\n\n"
                  "Trả `{ok, toast, closeForm, card, cardVersion}`: hiện `toast`; có `card` mới và "
                  "`cardVersion` **lớn hơn** thì thay card tại chỗ.\n\n"
                  "Lỗi nghiệp vụ: vẫn `200` nhưng `toast.type:\"error\"`, không có `card` → chỉ "
                  "hiện toast, giữ card cũ.\n\n"
                  "Cần `card_message_id` (bước 3.1 tự lấy tin có card đầu tiên).",
             tests=('pm.test("200 / 400 / 403 / 404",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403, 404]));',)),
        step("5.1", "Thông báo chưa đọc", "GET", "/api/client/v1/chat/notifications",
             query=[("unread", "true"), ("limit", "20")], auth=CMS_AUTH,
             desc="`element.unread` là **tổng** số chưa đọc — dùng cho badge, không đổi theo filter.\n\n"
                  "Mỗi thông báo có `messageId` để bấm vào là nhảy tới đúng card.",
             tests=(OK,)),
        step("5.2", "Đánh dấu đã đọc", "POST", "/api/client/v1/chat/notifications/read",
             body={"all": True}, auth=CMS_AUTH,
             desc="`{\"all\": true}` đánh dấu hết; `{\"ids\": [...]}` đánh dấu từng cái.",
             tests=(OK,)),
        step("6.1", "Đồng bộ lại nhóm (sau khi vào link mời)", "POST",
             "/api/client/v1/chat/groups/refresh", auth=CMS_AUTH,
             desc="Gọi **ngay sau** khi người dùng vào nhóm bằng link mời (FE đã gọi CMS "
                  "`POST /workspace/groups/join` trước đó) — để không phải chờ chu kỳ đồng bộ 60 giây.\n\n"
                  "Trả `{synced, groups}`: `groups` cùng hình dạng với `bootstrap` nên vẽ lại sidebar "
                  "được ngay. `synced` là tổng số nhóm CMS trả về, **không** phải số nhóm của bạn.\n\n"
                  "Bình thường không cần gọi định kỳ: Chat tự đồng bộ 60s/lần và phát event `reload` "
                  "khi nhóm có thay đổi thật.",
             tests=('pm.test("200 / 400 (chua bat CMS)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400]));',)),
    ],
)

flow_cms = folder(
    "FLOW · Luồng của CMS",
    "Thứ tự CMS gọi vào Chat, từ lúc có nhóm tới khi đơn xong. **Chạy lần lượt từ 1.1**.\n\n"
    "### Chuẩn bị\n"
    "Điền **`service_key`** = `CHAT_SERVICE_KEY` (khoá dùng chung hai phía). Mọi request trong "
    "collection này tự gắn header `X-Chat-Service-Key` + `Idempotency-Key` — không dùng JWT.\n\n"
    "### Dừng ở một bước\n"
    "Đặt `stop_at` = số bước (vd `3.1`) rồi Run collection.\n\n"
    "### Hai điều quan trọng nhất\n"
    "**1. `Idempotency-Key`** (UUID) cho mọi request ghi. Chat lưu `(key → kết quả)`; gặp lại key "
    "cũ thì trả nguyên kết quả lần đầu, **không làm lại việc** — kể cả khi body lần hai khác. "
    "Retry phải dùng **cùng key**.\n\n"
    "**2. `cardVersion` phải tăng đơn điệu.** Chat chỉ áp card mới khi `cardVersion` **lớn hơn** "
    "bản đang lưu; nhỏ hơn/bằng → `{ok, ignored:true}` và bỏ qua luôn `thread`/`notify`.\n\n"
    "Hợp đồng đầy đủ: `docs/cms-goi-chat.md`.",
    [
        step("1.1", "Chat đang giữ những nhóm nào?", "GET", "/bot/v1/groups",
             query=[("source", "cms"), ("q", "", True)],
             auth={"type": "noauth"},
             desc="Dùng để đối chiếu khi debug đồng bộ. `?q=` tìm theo tên, `?source=` = `cms` | `local`.\n\n"
                  "`id` là `chatId` của CMS, `internalId` là UUID của Chat — endpoint nào cũng nhận "
                  "cả hai dạng.",
             tests=(OK, 'const items = pm.response.json().items || [];',
                    'const g = items.find(x => String(x.id).startsWith("wg_")) || items[0];',
                    'if (g) pm.collectionVariables.set("group_id", g.id);',
                    'console.log("nhom =", items.map(i => i.id + " (" + i.memberCount + " nguoi)").join(", "));')),
        step("1.2", "Nhóm này có những ai?", "GET", "/bot/v1/groups/{{group_id}}/members",
             auth={"type": "noauth"},
             desc="`id` trả về là **actorId** (`u_68`) — định danh Chat cấp. Dùng cho `visibleTo` của "
                  "nút trên card và cho `notify[].actorId`.",
             tests=(OK, 'const items = pm.response.json().items || [];',
                    'if (items[0]) pm.collectionVariables.set("actor_id", items[0].id);',
                    'console.log("thanh vien =", items.map(i => i.id).join(", "));')),
        step("2.1", "openId → actorId", "POST", "/bot/v1/users/resolve",
             body={"items": [{"openId": "local:flow-cms-1", "cmsUserId": 9400,
                              "name": "Người mới"}]},
             auth={"type": "noauth"},
             desc="Gọi khi cần `actorId` mà chưa có. Người **chưa từng đăng nhập Chat** cũng dùng "
                  "được: Chat tạo actor từ hồ sơ CMS.\n\n"
                  "Một lần gọi nhiều người (Chat tra bằng 2 câu query chứ không phải N câu). "
                  "`actorId` ổn định theo `cmsUserId` nên **CMS nên cache**, không cần gọi lại.\n\n"
                  "Endpoint này **không cần** `Idempotency-Key` — nó vốn idempotent theo `openId`.",
             tests=(OK, 'pm.test("co items", () => pm.expect(pm.response.json().items).to.be.an("array"));')),
        step("2.2", "Đẩy nhóm + thành viên", "PUT", "/bot/v1/groups/{{cms_chat_id}}",
             body={"name": "Nhóm Thiết kế Web",
                   "members": [{"openId": "local:flow-cms-1", "cmsUserId": 9400,
                                "name": "Người mới", "sources": ["team"],
                                "teamCodes": ["DESIGN"]}]},
             auth={"type": "noauth"},
             desc="Tạo nhóm nếu chưa có, cập nhật tên, và **thay toàn bộ** danh sách thành viên: "
                  "ai không còn trong `members` **mất quyền xem nhóm ngay**.\n\n"
                  "Tham số URL phải là `chatId` dạng `wg_<hex>` — đây là id CMS muốn tạo, truyền UUID "
                  "vào sẽ đẻ ra nhóm lạ.\n\n"
                  "Chat chỉ bắn event `reload` khi **thật sự** có thay đổi:\n\n"
                  "| Thay đổi | `reason` | Gửi cho ai |\n|---|---|---|\n"
                  "| thêm/bớt thành viên | `group.members_changed` | thành viên hiện tại ∪ người vừa bị gỡ |\n"
                  "| đổi tên nhóm | `group.updated` | thành viên hiện tại |\n"
                  "| không đổi gì | *(không bắn)* | — |\n\n"
                  "Chạy request này 2 lần liên tiếp để thấy: lần hai không ai nhận gì.\n\n"
                  "Chat cũng tự `GET /workspace/groups` mỗi 60s nên nhóm cuối cùng vẫn khớp CMS kể cả "
                  "khi `PUT` thất bại — `PUT` chỉ để **nhanh hơn**.",
             tests=(OK, 'pm.test("co memberCount",'
                    ' () => pm.expect(pm.response.json()).to.have.property("memberCount"));')),
        step("3.1", "Đẩy card đơn vào nhóm", "POST", "/bot/v1/messages",
             body={"groupId": "{{group_id}}",
                   "bot": {"id": 1, "name": "Designer Bot"},
                   "card": {"schema": "abc.card/v1",
                            "header": {"code": "WEB-20261118-0006", "title": "Banner Tết 2027",
                                       "status": {"text": "Chưa nhận", "tone": "warning"}},
                            "elements": [{"type": "field", "label": "Số lượng", "value": "3"},
                                         {"type": "button", "text": "Nhận việc",
                                          "action": "accept_task",
                                          "visibleTo": ["{{actor_id}}"]}]}},
             auth={"type": "noauth"},
             desc="**Gọi ngay sau khi tạo đơn.** Đây là bước làm card hiện ra trong nhóm — "
                  "`form.submit` chỉ làm Chat ghi một tin system \"đã gửi yêu cầu\", **không** sinh card.\n\n"
                  "⚠️ Khoá của nút là **`action`**, KHÔNG phải `actionId`. Sai tên thì card vẫn tạo "
                  "được nhưng người bấm nhận `403 action_not_allowed`.\n\n"
                  "`visibleTo` = danh sách `actorId` được thấy nút. Chat lọc nút theo **từng người xem** "
                  "trước khi gửi xuống UI; để `null`/bỏ trống = ai cũng thấy.\n\n"
                  "**Lưu `messageId` trả về, ghép với mã đơn** — không có nó thì không `PATCH` được "
                  "card về sau.\n\n"
                  "Body cũng nhận `{groupId, text}` cho tin nhắn thường, và `threadRootId` để gửi vào thread.",
             tests=(OK, 'const d = pm.response.json();',
                    'pm.test("co messageId + cardVersion",'
                    ' () => pm.expect(d).to.include.keys("messageId", "cardVersion"));',
                    'pm.collectionVariables.set("card_message_id", d.messageId);')),
        step("3.2", "Ghi một dòng diễn biến", "POST",
             "/bot/v1/messages/{{card_message_id}}/thread",
             body={"text": "Đã giao Trần Designer · Chờ nhận task.",
                   "bot": {"id": 1, "name": "Designer Bot"}},
             auth={"type": "noauth"},
             desc="Dùng cho lịch sử đơn khi **chỉ** cần ghi chữ, không đổi card. Nếu vừa đổi card "
                  "vừa ghi dòng thì dùng bước 4.1 (gộp) thay vì gọi riêng.",
             tests=(OK, 'pm.test("co messageId",'
                    ' () => pm.expect(pm.response.json()).to.have.property("messageId"));')),
        step("3.3", "Thông báo cho một người", "POST", "/bot/v1/notifications",
             body={"actorId": "{{actor_id}}",
                   "text": "Bạn có task mới: WEB-20261118-0006 · Banner Tết 2027",
                   "messageId": "{{card_message_id}}"},
             auth={"type": "noauth"},
             desc="Chuông đỏ + badge của riêng người đó. `messageId` để FE bấm vào là nhảy tới đúng card.\n\n"
                  "Người dùng tự đánh dấu đã đọc qua bề mặt FE — CMS không cần gọi gì thêm.",
             tests=(OK, 'pm.test("co id", () => pm.expect(pm.response.json()).to.have.property("id"));')),
        step("4.1", "Đơn đổi trạng thái — gộp 3 việc 1 request", "PATCH",
             "/bot/v1/messages/{{card_message_id}}",
             body={"card": {"schema": "abc.card/v1",
                            "header": {"code": "WEB-20261118-0006", "title": "Banner Tết 2027",
                                       "status": {"text": "Đã nộp bài", "tone": "success"}},
                            "elements": [{"type": "text", "text": "Chờ duyệt"}]},
                   "cardVersion": 2,
                   "thread": {"text": "Trần Designer đã nộp bài · Chờ duyệt",
                              "bot": {"id": 1, "name": "Designer Bot"}},
                   "notify": [{"actorId": "{{actor_id}}",
                               "text": "Đơn WEB-20261118-0006 đã nộp bài"}]},
             auth={"type": "noauth"},
             desc="**Dùng request này cho MỖI lần đơn đổi trạng thái** (nhận / tạm dừng / treo / "
                  "nộp bài / xong / huỷ) — thay cho 3 request riêng. Một `Idempotency-Key`, một chỗ "
                  "xử lý lỗi.\n\n"
                  "`card` + `cardVersion` bắt buộc; `thread` và `notify` **tuỳ chọn** (không gửi thì "
                  "response không có khoá đó). `notify` là mảng, gửi nhiều người một lần được; "
                  "`notify[].messageId` bỏ trống thì tự trỏ về chính card này.\n\n"
                  "**Quy ước lỗi:**\n"
                  "- `cardVersion` không lớn hơn → `{ok, ignored:true}`, **bỏ qua cả** `thread`/`notify`\n"
                  "- `thread`/`notify` lỗi → vẫn `200`, card đã đổi xong, lỗi theo từng phần: "
                  "`{\"thread\":{\"error\":{\"code\":\"text_required\"}}}`\n\n"
                  "Không trả 5xx cho phần phụ vì CMS retry sẽ gặp `ignored:true` (cardVersion không "
                  "tăng nữa) và mất luôn thread/notify.",
             tests=(OK, 'const d = pm.response.json();',
                    'console.log("ignored =", d.ignored, "| thread =", JSON.stringify(d.thread),',
                    '            "| notify =", JSON.stringify(d.notify));',
                    'pm.test("ignored thi khong co thread/notify", () => {',
                    '  if (d.ignored) { pm.expect(d).to.not.have.property("thread");',
                    '                   pm.expect(d).to.not.have.property("notify"); }',
                    '});')),
        step("4.2", "Gửi lại cùng cardVersion → ignored", "PATCH",
             "/bot/v1/messages/{{card_message_id}}",
             body={"card": {"schema": "abc.card/v1",
                            "header": {"title": "Bản cũ hơn"}, "elements": []},
                   "cardVersion": 2,
                   "thread": {"text": "Dòng này KHÔNG được xuất hiện"}},
             auth={"type": "noauth"},
             desc="Chứng minh cơ chế chống ghi đè ngược: `cardVersion` **bằng** bản đang lưu nên "
                  "Chat bỏ qua, và `thread` cũng không được ghi.\n\n"
                  "Response: `{ok: true, ignored: true, cardVersion: 2}` — **không** có khoá "
                  "`thread`.\n\n"
                  "`ignored: true` **không phải lỗi**: nghĩa là Chat đang giữ bản mới hơn. CMS đọc "
                  "`cardVersion` trả về để biết trạng thái thật ở Chat.",
             tests=(OK, 'const d = pm.response.json();',
                    'pm.test("bi bo qua", () => pm.expect(d.ignored).to.be.true);',
                    'pm.test("khong ghi thread", () => pm.expect(d).to.not.have.property("thread"));')),
        step("5.1", "Ghim card quan trọng", "POST",
             "/bot/v1/messages/{{card_message_id}}/pin", body={},
             auth={"type": "noauth"},
             desc="`{}` = ghim, `{\"pinned\": false}` = bỏ ghim. Card hiện ở mục đã ghim của nhóm.",
             tests=(OK, 'pm.test("ok", () => pm.expect(pm.response.json().ok).to.be.true);')),
        step("5.2", "Cập nhật presence (lịch OFF)", "PUT", "/bot/v1/presence",
             body={"items": [{"actorId": "{{actor_id}}", "status": "off",
                              "reason": "Lịch OFF ca chiều"}]},
             auth={"type": "noauth"},
             desc="| `status` | `reason` | Chat hiểu là |\n|---|---|---|\n"
                  "| `on` | — | online |\n"
                  "| `off` | **có** | `off_duty` → FE hiện chữ **OFF** cạnh tên |\n"
                  "| `off` | không | offline |\n\n"
                  "`actorId` không tồn tại thì **bị bỏ qua, không báo lỗi** — `updated` cho biết "
                  "thực sự đổi được bao nhiêu.",
             tests=(OK, 'pm.test("co updated",'
                    ' () => pm.expect(pm.response.json().updated).to.be.a("number"));')),
        step("6.1", "Tra cứu nhiều user một lần", "POST", "/bot/v1/users/batch",
             body={"actorIds": ["{{actor_id}}"], "openIds": [], "cmsUserIds": []},
             auth={"type": "noauth"},
             desc="Mỗi loại id **một câu query** (không phải N câu). Dùng khi cần đối chiếu hồ sơ.\n\n"
                  "Tra một người: `GET /bot/v1/users/{actorId|openId|UUID}`.",
             tests=(OK, 'pm.test("co items",'
                    ' () => pm.expect(pm.response.json()).to.have.property("items"));')),
    ],
)


def make_collection(cid, name, desc, items):
    """Dung mot collection dung chung prerequest/variable voi ban day du."""
    return {
        "info": {"_postman_id": cid, "name": name, "description": desc, "schema": SCHEMA},
        "auth": collection["auth"],
        "event": collection["event"],
        "variable": collection["variable"],
        "item": items,
    }


FE_DESC = """\
# Chat Service — bản cho **FE**

Chỉ gồm những API **web chat gọi**: `/api/client/v1/chat/*` + WebSocket.
Phần CMS gọi sang Chat nằm ở bản `[CMS]`.

## Chuẩn bị (2 bước)

1. Chọn environment **Chat Service · Local**.
2. Điền biến **`cms_token`** = access token do CMS phát.
   Chat **không có login**, không phát token riêng — token lấy từ
   `POST /api/client/v1/workspace/auth/login` của CMS.
   Local: `make token-cms CMS_USER=6`.

## Bắt đầu từ đâu

Chạy folder **`FLOW · Luồng của FE`** từ `1.1` xuống — mỗi bước giải thích gọi khi nào và
response nói gì. Đặt biến `stop_at` = số bước để dừng lại xem kỹ.

## Hình dạng response

```json
{ "statusCode": 200, "code": "SUCCESS", "message": "Success", "element": { ... } }
```

Lỗi: `statusCode` ≠ 200 và `code` là mã ổn định (`NOT_CONVERSATION_MEMBER`,
`ACTION_NOT_ALLOWED`…) — bắt theo `code`, đừng bắt theo `message`.

Field dạng **camelCase**: `text`, `createdAt`, `threadRootId`, `clientMsgId`, `unreadCount`.
WebSocket trả **cùng tên field** nên FE chỉ cần một cách đọc.

Phân trang: cursor (`?cursor=<pageInfo.nextCursor>&limit=30`), **không** dùng offset.

## Ba điều dễ sai nhất

1. **`ok: true` không có nghĩa là thành công về nghiệp vụ.** Với form và card action, đọc
   `toast.type` và `closeForm`.
2. **Card đơn không đến từ response của form submit.** Nó tới qua realtime khi CMS đẩy card
   vào nhóm — nghe event `message` và upsert theo `id`.
3. **`{id}` của nhóm nhận cả `wg_…` lẫn UUID.** Dùng giá trị nào `bootstrap` trả về cũng được.
"""

CMS_DESC = """\
# Chat Service — bản cho **CMS**

Chỉ gồm những API **CMS gọi sang Chat** (`/bot/v1/*`). Phần web chat gọi nằm ở bản `[FE]`.

## Chuẩn bị (1 bước)

Chọn environment **Chat Service · Local** rồi điền **`service_key`** = `CHAT_SERVICE_KEY`
(khoá dùng chung hai phía, cũng là khoá Chat dùng để ký webhook gửi sang CMS).

Collection tự gắn `X-Chat-Service-Key` và `Idempotency-Key` cho mọi request `/bot/v1`.

## Bắt đầu từ đâu

Chạy folder **`FLOW · Luồng của CMS`** từ `1.1` xuống. Đặt `stop_at` = số bước để dừng xem kỹ.

## Quy ước chung

- **Không có envelope** — response là JSON phẳng (`{"messageId": "..."}`), khác bản `[FE]`.
- **`Idempotency-Key`** (UUID) cho mọi request ghi: `POST /messages`, `PATCH /messages/:id`,
  `POST /messages/:id/thread`, `POST /messages/:id/pin`, `POST /notifications`,
  `PUT /presence`, `PUT /groups/:id`. Gặp lại key cũ → trả nguyên kết quả lần đầu.
  Retry phải dùng **cùng key**.
- **Lỗi**: `{"code", "message", "retryable"}`. `retryable: true` (5xx) thì thử lại một lần với
  cùng key; `false` thì sửa payload.
- **Id nhóm** nhận `wg_<hex>` hoặc UUID nội bộ. Ngoại lệ: `PUT /groups/:id` bắt buộc `wg_<hex>`
  vì đó là chatId cần tạo.

## Ba điều dễ sai nhất

1. **`cardVersion` phải tăng đơn điệu.** Nhỏ hơn/bằng bản đang lưu → `{ok, ignored:true}`,
   card không đổi và `thread`/`notify` bị bỏ qua.
2. **Khoá của nút là `action`, không phải `actionId`.** Sai tên thì card tạo được nhưng người
   bấm nhận `403 action_not_allowed`.
3. **Lưu `messageId`** mà `POST /messages` trả về, ghép với mã đơn — không có nó thì không
   `PATCH` được card về sau.

Hợp đồng đầy đủ kèm ví dụ trọn vòng: `docs/cms-goi-chat.md`.
"""

collection_fe = make_collection(
    FE_ID, "Chat Service · A Workspace [FE]", FE_DESC,
    [flow_fe, cms_client, websocket, start, conversations, messages, errors],
)

collection_cms = make_collection(
    CMS_ID, "Chat Service · A Workspace [CMS]", CMS_DESC,
    [flow_cms, cms_bot],
)


def check():
    """Kiem 3 file JSON: cau truc hop le + khong lech so voi script sinh ra no.

    Thay cho `postman collection lint` (lenh do chi doc duoc dinh dang v3,
    quet file v2.1 ra 0 ket qua nen khong noi len dieu gi)."""
    errs, warns = [], []
    total_folders = total_reqs = 0

    for path, col in OUTPUTS:
        label = col["info"]["name"]
        if not path.exists():
            errs.append(f"thieu {path.relative_to(ROOT)}")
            continue
        on_disk = json.loads(path.read_text(encoding="utf-8"))

        # 1) cau truc
        seen = set()
        for folder_ in on_disk.get("item", []):
            if not folder_.get("name"):
                errs.append(f"[{label}] co folder khong ten")
            total_folders += 1
            for req_ in folder_.get("item", []):
                total_reqs += 1
                name = f"[{label}] {folder_['name']} / {req_.get('name', '?')}"
                if name in seen:
                    errs.append(f"trung ten request: {name}")
                seen.add(name)
                r = req_.get("request") or {}
                if not r.get("method"):
                    errs.append(f"{name}: thieu method")
                raw = ((r.get("url") or {}).get("raw") or "")
                if not raw:
                    errs.append(f"{name}: thieu URL")
                elif "{{base_url}}" not in raw and "{{ws_" not in raw:
                    warns.append(f"{name}: URL khong dung {{{{base_url}}}}")
                if r.get("method") in ("POST", "PUT", "PATCH") and (r.get("body") or {}).get("mode") == "raw":
                    try:
                        json.loads(((r.get("body") or {}).get("raw") or "{}"))
                    except json.JSONDecodeError as e:
                        errs.append(f"{name}: body khong phai JSON hop le ({e})")

        # 2) lech voi script
        fresh = json.dumps(col, ensure_ascii=False, indent=2) + "\n"
        if fresh != path.read_text(encoding="utf-8"):
            errs.append(f"[{label}] file tren dia LECH voi scripts/gen_postman.py — "
                        f"chay `make postman` de sinh lai")

    # 3) hai ban tach phai la tap con cua ban day du (khong de request "mo coi")
    full_names = {
        f"{f['name']} / {r['name']}"
        for f in collection["item"] for r in f["item"]
    }
    for _, col in OUTPUTS[1:]:
        for f in col["item"]:
            if f["name"].startswith("FLOW"):
                continue  # FLOW cua ban tach viet rieng, co y khac
            for r in f["item"]:
                key = f"{f['name']} / {r['name']}"
                if key not in full_names:
                    errs.append(f"[{col['info']['name']}] request khong co trong ban day du: {key}")

    for w in warns:
        print(f"! {w}")
    for e in errs:
        print(f"✗ {e}")
    if errs:
        return 1
    print(f"  {len(OUTPUTS)} file | {total_folders} folder | {total_reqs} request")
    for _, col in OUTPUTS:
        n = sum(len(f["item"]) for f in col["item"])
        print(f"  - {col['info']['name']}: {len(col['item'])} folder, {n} request")
    print("✓ ca 3 collection JSON hop le")
    return 0



# (file, collection) — ban day du + 2 ban tach cho tung doi.
OUTPUTS = [
    (COLLECTION_FILE, collection),
    (FE_FILE, collection_fe),
    (CMS_FILE, collection_cms),
]


def main():
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    for path, col in OUTPUTS:
        path.write_text(json.dumps(col, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        n = sum(len(f["item"]) for f in col["item"])
        print(f"✓ {path.relative_to(ROOT)} ({len(col['item'])} folder, {n} request) — {col['info']['name']}")
    ENV_FILE.write_text(json.dumps(environment, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"✓ {ENV_FILE.relative_to(ROOT)}")


if __name__ == "__main__":
    if "--check" in sys.argv:
        raise SystemExit(check())
    main()
