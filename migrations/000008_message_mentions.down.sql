-- Xoa du lieu nhac ten cua moi tin nhan (khong khoi phuc duoc).
DROP INDEX IF EXISTS ix_messages_mentions;
ALTER TABLE messages DROP COLUMN IF EXISTS mention_all;
ALTER TABLE messages DROP COLUMN IF EXISTS mentions;
