-- Ghim nhom len dau thanh ben — rieng tung nguoi (moi thanh vien tu ghim, khong anh huong nguoi khac).
-- NULL = khong ghim; co gia tri = thoi diem ghim (ghim gan nhat xep tren cung).
ALTER TABLE conversation_members ADD COLUMN IF NOT EXISTS pinned_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS ix_conv_members_pinned
    ON conversation_members (user_id, pinned_at DESC) WHERE pinned_at IS NOT NULL;
