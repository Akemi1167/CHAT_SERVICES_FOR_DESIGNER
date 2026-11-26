-- Nhac ten (@) trong tin nhan.
-- mentions    = mang actorId (u_<cmsUserId>; user khong co actorId thi la UUID noi bo)
--               cua nhung nguoi duoc nhac. @all da duoc mo rong thanh moi thanh vien.
-- mention_all = nguoi gui dung @all (de UI ve chip @all).
ALTER TABLE messages ADD COLUMN IF NOT EXISTS mentions    JSONB;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS mention_all BOOLEAN NOT NULL DEFAULT false;

-- Dem tin chua doc nhac toi minh (dau "@" tren danh sach nhom): mentions @> '["u_55"]'.
CREATE INDEX IF NOT EXISTS ix_messages_mentions
    ON messages USING gin (mentions jsonb_path_ops) WHERE mentions IS NOT NULL;
