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
# `collections/` nua — xem docs/09-postman.md.
OUT_DIR = ROOT / "internal" / "postman"
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
            body={"groupId": "{{group_id}}", "bot": {"id": 1, "name": "Demo Designer Bot"},
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
        req("Đẩy nhóm + thành viên", "PUT", "/bot/v1/groups/{{cms_chat_id}}",
            body={"name": "Nhóm Thiết kế Web",
                  "members": [{"openId": "local:designer-1", "cmsUserId": 68, "name": "Trần Designer",
                               "sources": ["team"], "teamCodes": ["DESIGN"]}]},
            desc="**Thay thế toàn bộ** danh sách thành viên: ai không còn trong danh sách mất quyền xem nhóm ngay.",
            auth={"type": "noauth"},
            tests=(OK, 'pm.test("co memberCount", () => pm.expect(pm.response.json()).to.have.property("memberCount"));')),
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
                 "kèm `members` + `sources`/`teamCodes`), `notifications`, `unreadNotifications`.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK, 'const el = pm.response.json().element;',
                   'pm.test("co me + groups", () => pm.expect(el).to.have.keys(["me","groups","notifications","unreadNotifications"]));',
                   'if (el.groups && el.groups[0]) pm.collectionVariables.set("group_id", el.groups[0].chatId || el.groups[0].id);')),
        req("Tôi là ai", "GET", "/api/client/v1/chat/auth/me",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=(OK,)),
        req("Đồng bộ lại nhóm từ CMS", "POST", "/api/client/v1/chat/groups/refresh",
            desc="Gọi sau khi user vào nhóm bằng link mời.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (OK) / 400 (chua bat CMS) / 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),
        req("Lịch sử tin nhắn (card đã lọc)", "GET", "/api/client/v1/chat/groups/{{group_id}}/messages",
            query=[("limit", "30")],
            desc="Card trả về đã **lọc nút theo `visibleTo`** của chính người gọi; `actions` liệt kê "
                 "action người đó được bấm.\n\n⚠️ `cms_token` phải là người **thuộc nhóm** `group_id`, "
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
        req("Phản hồi của 1 tin nhắn", "GET", "/api/client/v1/chat/messages/{{card_message_id}}/replies",
            query=[("mode", "thread"), ("limit", "50")],
            desc="Trả `root`, `replies` (card đã lọc `visibleTo`), `replyCount`, `threadCount`. "
                 "`mode=direct` cho trích dẫn, `mode=thread` cho panel thread của card.",
            auth={"type": "bearer", "bearer": [{"key": "token", "value": "{{cms_token}}", "type": "string"}]},
            tests=('pm.test("200 (token thuoc nhom) hoac 403 (token khong thuoc nhom)",'
                   ' () => pm.expect(pm.response.code).to.be.oneOf([200, 201, 403]));',)),
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
            body={"values": {"kind": "task", "title": "Banner Tết 2027", "teamId": "27", "useAI": False},
                  "context": {"groupId": "{{group_id}}"}},
            desc="FE gửi `teamId` trong `values`; Chat **không diễn giải**, chuyển nguyên sang CMS "
                 "trong event `form.submit` rồi trả `toast`/`closeForm` cho UI.",
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

Hai loại request này **không nằm trong file JSON** (định dạng v2.1 của Postman không chứa được),
chạy bằng Postman CLI: `postman collection run "postman/collections/Chat Service"`.
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
    "Toàn bộ vòng đời một nhóm chat, theo đúng thứ tự thật. **Chạy lần lượt từ 1.1**.\n\n"
    "### Dừng ở một bước\n"
    "Đặt biến `stop_at` = số bước (vd `2.3`) rồi **Run collection** → chạy tới bước đó thì dừng. "
    "Xem response, đổi `stop_at` sang bước sau để đi tiếp. Để trống = chạy hết.\n\n"
    "### Đánh số\n"
    "`1.1`, `1.2`… = các bước **cùng cấp** trong một giai đoạn. Giai đoạn:\n"
    "1 · chuẩn bị · 2 · nhóm & thành viên · 3 · tin nhắn · 4 · card và form (CMS) · "
    "5 · thông báo · 6 · dọn dẹp.\n\n"
    "### Cần gì trước\n"
    "- `token` — `make token SUB=demo-an NAME=\"An\" ROLE=order`\n"
    "- `service_key` — = `CHAT_SERVICE_KEY` trong `.env` (chỉ cho bước 2.x, 4.x)\n"
    "- `cms_token` — token của người **thuộc nhóm**; thiếu thì để bằng `token`\n\n"
    "Biến `group_id` được bước 2.1 lưu lại, các bước sau dùng chung — không phải điền tay.",
    [
        # ---- 1 · chuan bi ----
        step("1.1", "Service còn sống?", "GET", "/healthz",
             desc="Chưa 200 thì không cần đi tiếp: `make dev-up` rồi `make dev-logs`.",
             tests=(OK, 'pm.test("status ok", () => pm.expect(pm.response.json().data.status).to.eql("ok"));')),
        step("1.2", "Tôi là ai", "GET", "/api/v1/me",
             desc="Xác minh `token` hợp lệ và lấy `user_id`. 401 ở đây = token sai hoặc "
                  "`JWT_SECRET` của app khác với lúc sinh token.",
             tests=(OK, *save("user_id", "_d.user_id"))),

        # ---- 2 · nhom & thanh vien ----
        step("2.1", "Tạo nhóm", "POST", "/api/v1/conversations",
             body={"type": "group", "name": "FLOW · nhóm thử", "member_user_ids": []},
             desc="Lưu **`group_id`** — biến dùng xuyên suốt cả 3 bề mặt API.\n\n"
                  "Đang có CMS thật? Bỏ qua bước này, chạy 2.2 để CMS đẩy nhóm sang.",
             tests=(OK, *save("group_id", "_d.id"))),
        step("2.2", "CMS đẩy nhóm + thành viên sang Chat", "PUT", "/bot/v1/groups/{{cms_chat_id}}",
             body={"name": "FLOW · nhóm từ CMS",
                   "members": [{"openId": "local:flow-1", "cmsUserId": 9001, "name": "Flow User",
                                "sources": ["team"], "teamCodes": ["DESIGN"]}]},
             auth={"type": "noauth"},
             desc="Đây là **chiều CMS → Chat**. Tham số URL là `cms_chat_id` (dạng `wg_<hex>`), "
                  "KHÔNG phải `group_id` — vì nó chính là chatId mà CMS muốn tạo.\n\n"
                  "**Thay thế toàn bộ** danh sách thành viên: ai không có trong `members` mất quyền xem ngay.\n\n"
                  "Cần `service_key`. Chưa bật CMS (`CMS_ENABLED=false`) thì bước này 404 — bỏ qua.\n\n"
                  "Bước này **không** đổi `group_id`: nhóm vừa đẩy chỉ có thành viên do CMS "
                  "khai, token của bạn không nằm trong đó nên các bước 3.x sẽ bị 403. "
                  "Nhóm ở 2.1 (bạn là owner) mới là nhóm dùng xuyên suốt.",
             tests=('pm.test("200 (da bat CMS) / 404 (CMS_ENABLED=false)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',)),
        step("2.3", "Nhóm vừa đẩy có những ai?", "GET", "/bot/v1/groups/{{cms_chat_id}}/members",
             auth={"type": "noauth"},
             desc="**Điểm dừng hay dùng**: xác nhận CMS đẩy thành viên sang đúng chưa.\n\n"
                  "`id` trả về là **actorId** (`u_9001`) — định danh Chat cấp cho CMS, không phải UUID. "
                  "Bước này lưu `actor_id` cho 5.1.\n\n"
                  "Muốn xem thành viên nhóm của 2.1 thì đổi URL sang `{{group_id}}` — cùng endpoint, "
                  "cả hai dạng id đều nhận.",
             tests=('pm.test("200 / 404 (chua bat CMS)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',
                    'const _it = pm.response.code === 200 && pm.response.json().items[0];',
                    'if (_it) pm.collectionVariables.set("actor_id", _it.id);')),
        step("2.4", "Nhóm của tôi (bề mặt FE)", "GET", "/api/client/v1/chat/bootstrap",
             auth=CMS_AUTH,
             desc="Tất cả thứ FE cần khi mở app. `groups` **chỉ gồm nhóm mình là thành viên** — "
                  "không thấy nhóm vừa tạo ở 2.2 là đúng, vì bạn không nằm trong `members` của nó.",
             tests=(OK,)),

        # ---- 3 · tin nhan ----
        step("3.1", "Gửi tin nhắn", "POST", "/api/v1/conversations/{{group_id}}/messages",
             body={"content": "FLOW: tin nhắn đầu tiên", "client_msg_id": "{{$guid}}"},
             desc="`client_msg_id` giúp chống gửi trùng khi mạng chập chờn.",
             tests=(OK, *save("message_id", "_d.message.id"))),
        step("3.2", "Trả lời trong thread", "POST", "/api/v1/conversations/{{group_id}}/messages",
             body={"content": "FLOW: trả lời", "reply_to_id": "{{message_id}}", "client_msg_id": "{{$guid}}"},
             desc="`reply_to_id` tạo thread. Tin gốc giữ nguyên, tin này có `thread_id` = id tin gốc.",
             tests=(OK, *save("reply_message_id", "_d.message.id"))),
        step("3.3", "Đọc lịch sử", "GET", "/api/v1/conversations/{{group_id}}/messages",
             query=[("limit", "30"), ("only_root", "true", True)],
             desc="Mới nhất trước, phân trang bằng `cursor`. Bật `only_root=true` để ẩn tin trong thread.",
             tests=(OK,)),
        step("3.4", "Đánh dấu đã đọc", "POST", "/api/v1/conversations/{{group_id}}/read",
             body={"message_id": "{{message_id}}"},
             desc="Trả `unread_count` mới. Realtime phát event `read` cho người khác trong nhóm.",
             tests=(OK,)),

        # ---- 4 · card & form (CMS) ----
        step("4.1", "CMS gửi card vào nhóm", "POST", "/bot/v1/messages",
             body={"groupId": "{{group_id}}", "card": {
                 "schema": "abc.card/v1", "title": "FLOW · Đơn thử",
                 "elements": [{"type": "text", "text": "Nội dung đơn"},
                              {"type": "button", "text": "Nhận việc", "action": "accept_task"}]}},
             auth={"type": "noauth"},
             desc="Chiều **CMS → Chat**. Card theo schema `abc.card/v1`.\n\n"
                  "⚠️ Khoá của nút là **`action`**, KHÔNG phải `actionId` — dùng sai tên thì "
                  "`POST /messages/:id/actions` trả `403 action_not_allowed`.\n\n"
                  "Nút có `visibleTo` được lọc theo từng người xem.",
             tests=('pm.test("200 / 404 (chua bat CMS)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 404]));',
                    'if (pm.response.code === 200) pm.collectionVariables.set("card_message_id",'
                    ' pm.response.json().messageId);')),
        step("4.2", "FE gửi form tạo yêu cầu", "POST", "/api/client/v1/chat/forms/create_task/submit",
             body={"values": {"kind": "task", "title": "FLOW · Banner", "teamId": "27", "useAI": False},
                   "context": {"groupId": "{{group_id}}"}},
             auth=CMS_AUTH,
             desc="Chiều **FE → Chat → CMS**. Chat **không diễn giải** `values`, chỉ ký HMAC rồi "
                  "chuyển nguyên sang CMS.\n\n"
                  "⚠️ `teamId` ở đây là **tổ của CMS**, dạng số (vd `27`) — không phải UUID, và không "
                  "liên quan gì tới bảng `teams` của module task nội bộ.",
             tests=('pm.test("200 / 400 (chua bat CMS) / 403 (khong thuoc nhom)",'
                    ' () => pm.expect(pm.response.code).to.be.oneOf([200, 400, 403]));',)),

        # ---- 5 · thong bao ----
        step("5.1", "CMS tạo thông báo cho 1 người", "POST", "/bot/v1/notifications",
             body={"actorId": "{{actor_id}}", "text": "FLOW: bạn có việc mới"},
             auth={"type": "noauth"},
             desc="Cần `actor_id`. Chưa có thì lấy ở 2.3 (`items[].id`).",
             tests=('pm.test("200 / 400 / 404", () => pm.expect(pm.response.code)'
                    '.to.be.oneOf([200, 400, 404]));',)),
        step("5.2", "Thông báo chưa đọc của tôi", "GET", "/api/client/v1/chat/notifications",
             query=[("unread", "true")],
             auth=CMS_AUTH,
             desc="`element.unread` là **tổng** số chưa đọc (cho badge), không đổi theo filter.",
             tests=(OK,)),
        step("5.3", "Đánh dấu đã đọc hết", "POST", "/api/client/v1/chat/notifications/read",
             body={"all": True}, auth=CMS_AUTH,
             desc="Trả số dòng vừa đánh dấu.", tests=(OK,)),

        # ---- 6 · don dep ----
        step("6.1", "Xoá tin nhắn vừa gửi", "DELETE", "/api/v1/messages/{{message_id}}",
             desc="Xoá mềm: tin còn trong DB nhưng FE không hiện. Bỏ qua nếu muốn giữ để xem tiếp.",
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

### 2.6 typing - dang nhap

```json
{
  "id": "c5",
  "type": "typing",
  "payload": {"conversationId": "{{group_id}}", "typing": true}
}
```
Khong luu DB, **khong co ack**. Chi nhung nguoi khac trong nhom nhan event `typing`.

### 2.7 message.delete - xoa mem

```json
{"id":"c6","type":"message.delete","payload":{"messageId":"{{message_id}}"}}
```

### 2.8 template.send - gui mau cho bot

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
  "card":{...},"cardVersion":2,"actions":["accept_task"]}}
```
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
| `notification` | CMS tao thong bao cho minh | `id`, `text`, `read`, `createdAt`, `messageId` |
| `user` | ho so nguoi dung doi | ho so dang rut gon |
| `reload` | nhom doi -> tai lai danh sach | `reason` (`group.members_changed` khi them/bot thanh vien, `group.updated` khi doi ten/cms_info), `conversationId`, `chatId`. **Khong doi gi thi khong ban** — khong phai moi 60s mot lan |

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
        {"key": "cms_token", "value": "", "type": "string"},
        {"key": "group_id", "value": "", "type": "string"},
        {"key": "cms_chat_id", "value": "wg_postman_demo", "type": "string"},
        {"key": "stop_at", "value": "", "type": "string"},
        {"key": "actor_id", "value": "", "type": "string"},
        {"key": "card_message_id", "value": "", "type": "string"},
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


def check():
    """Kiem tra file JSON: cau truc hop le + khong lech so voi script sinh ra no.

    Thay cho `postman collection lint` (lenh do chi doc duoc dinh dang v3,
    quet file v2.1 ra 0 ket qua nen khong noi len dieu gi)."""
    errs, warns = [], []

    if not COLLECTION_FILE.exists():
        print(f"✗ thieu {COLLECTION_FILE}")
        return 1
    on_disk = json.loads(COLLECTION_FILE.read_text(encoding="utf-8"))

    # 1) cau truc
    seen = set()
    for folder in on_disk.get("item", []):
        if not folder.get("name"):
            errs.append("co folder khong ten")
        for req in folder.get("item", []):
            name = f"{folder['name']} / {req.get('name', '?')}"
            if name in seen:
                errs.append(f"trung ten request: {name}")
            seen.add(name)
            r = req.get("request") or {}
            if not r.get("method"):
                errs.append(f"{name}: thieu method")
            raw = ((r.get("url") or {}).get("raw") or "")
            if not raw:
                errs.append(f"{name}: thieu URL")
            elif "{{base_url}}" not in raw:
                warns.append(f"{name}: URL khong dung {{{{base_url}}}}")
            if r.get("method") in ("POST", "PUT", "PATCH") and (r.get("body") or {}).get("mode") == "raw":
                try:
                    json.loads(((r.get("body") or {}).get("raw") or "{}"))
                except json.JSONDecodeError as e:
                    errs.append(f"{name}: body khong phai JSON hop le ({e})")

    # 2) lech voi script
    fresh = json.dumps(collection, ensure_ascii=False, indent=2) + "\n"
    if fresh != COLLECTION_FILE.read_text(encoding="utf-8"):
        errs.append("file tren dia LECH voi scripts/gen_postman.py — chay `make postman` de sinh lai")

    n = sum(len(f["item"]) for f in on_disk.get("item", []))
    print(f"  {len(on_disk.get('item', []))} folder, {n} request, {len(seen)} ten duy nhat")
    for w in warns:
        print(f"  ! {w}")
    for e in errs:
        print(f"  ✗ {e}")
    print("✓ collection JSON hop le" if not errs else f"✗ {len(errs)} loi")
    return 1 if errs else 0


# Folder viet tay trong ban v3 (v2.1 khong chua duoc 2 protocol nay) — khong ghi de.

def main():
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    COLLECTION_FILE.write_text(json.dumps(collection, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    ENV_FILE.write_text(json.dumps(environment, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    n = sum(len(f["item"]) for f in collection["item"])
    print(f"✓ {COLLECTION_FILE.relative_to(ROOT)} ({len(collection['item'])} folder, {n} request)")
    print(f"✓ {ENV_FILE.relative_to(ROOT)}")


if __name__ == "__main__":
    if "--check" in sys.argv:
        raise SystemExit(check())
    main()
