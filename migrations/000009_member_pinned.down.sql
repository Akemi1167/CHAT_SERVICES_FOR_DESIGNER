-- Xoa trang thai ghim nhom cua moi nguoi (khong khoi phuc duoc).
DROP INDEX IF EXISTS ix_conv_members_pinned;
ALTER TABLE conversation_members DROP COLUMN IF EXISTS pinned_at;
