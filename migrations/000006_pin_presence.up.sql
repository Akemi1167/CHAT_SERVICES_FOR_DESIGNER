-- Ghim tin nhan + ly do presence (phuc vu Bot API: pin, presence).

ALTER TABLE messages ADD COLUMN IF NOT EXISTS pinned     BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS pinned_at  TIMESTAMPTZ;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS pinned_by  UUID REFERENCES users(id) ON DELETE SET NULL;

-- Danh sach ghim cua mot nhom (thuong rat it dong nen index partial la du).
CREATE INDEX IF NOT EXISTS ix_messages_pinned
    ON messages (conversation_id, pinned_at DESC) WHERE pinned AND deleted_at IS NULL;

-- Ly do khi OFF: "lich OFF", "het ca", ... CMS gui kem qua PUT /bot/v1/presence.
ALTER TABLE users ADD COLUMN IF NOT EXISTS presence_reason VARCHAR(128) NOT NULL DEFAULT '';
