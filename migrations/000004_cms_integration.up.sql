-- Tich hop CMS: CMS la nguon su that cho user / nhom / don (card).
-- Chat la nguon su that cho actorId, tin nhan, cardVersion, trang thai doc, presence.

-- ---------- users: dinh danh phia CMS ----------
ALTER TABLE users ADD COLUMN IF NOT EXISTS cms_user_id  BIGINT;
ALTER TABLE users ADD COLUMN IF NOT EXISTS open_id      VARCHAR(128);
-- actor_id la dinh danh Chat tra ve cho CMS (mock CMS dung dang u_<cmsUserId>).
ALTER TABLE users ADD COLUMN IF NOT EXISTS actor_id     VARCHAR(64);
ALTER TABLE users ADD COLUMN IF NOT EXISTS username     VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS status       VARCHAR(32)  NOT NULL DEFAULT 'active';
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_designer  BOOLEAN      NOT NULL DEFAULT false;
-- Ho so hien thi lay tu CMS (webTeams / assistantOf / botAdminOf) de khoi goi lai nhieu lan.
ALTER TABLE users ADD COLUMN IF NOT EXISTS cms_profile  JSONB;
ALTER TABLE users ADD COLUMN IF NOT EXISTS synced_at    TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_cms_user_id ON users (cms_user_id) WHERE cms_user_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_open_id     ON users (open_id)     WHERE open_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_users_actor_id    ON users (actor_id)    WHERE actor_id IS NOT NULL;

-- ---------- conversations: nhom Web do CMS quan ly ----------
-- external_id = chatId cua CMS (wg_<hex>). Nhom tao trong Chat thi NULL.
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS external_id VARCHAR(64);
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS source      VARCHAR(16) NOT NULL DEFAULT 'local'; -- local | cms
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS cms_info    JSONB;  -- botId, teams, orderTeamCodes, timezone...
ALTER TABLE conversations ADD COLUMN IF NOT EXISTS synced_at   TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS ux_conversations_external_id
    ON conversations (external_id) WHERE external_id IS NOT NULL;

-- ---------- conversation_members: nguon thanh vien do CMS tinh ----------
ALTER TABLE conversation_members ADD COLUMN IF NOT EXISTS sources    JSONB; -- ["team","assistant","manual","invite"]
ALTER TABLE conversation_members ADD COLUMN IF NOT EXISTS team_codes JSONB; -- ["DESIGN","VIDEO"]

-- ---------- messages: card abc.card/v1 ----------
ALTER TABLE messages ADD COLUMN IF NOT EXISTS card         JSONB;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS card_version INTEGER NOT NULL DEFAULT 0;
-- bot_sender = {"id":1,"name":"Demo Designer Bot"} do CMS gui kem.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS bot_sender   JSONB;

CREATE INDEX IF NOT EXISTS ix_messages_card ON messages (conversation_id) WHERE card IS NOT NULL;

-- ---------- notifications (CMS day qua Bot API) ----------
CREATE TABLE IF NOT EXISTS notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    text       TEXT NOT NULL,
    message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    payload    JSONB,
    read       BOOLEAN NOT NULL DEFAULT false,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_notifications_user ON notifications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_notifications_unread ON notifications (user_id) WHERE NOT read;

-- ---------- idempotency cho Bot API (CMS gui Idempotency-Key) ----------
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key         VARCHAR(128) PRIMARY KEY,
    endpoint    VARCHAR(128) NOT NULL,
    status_code INTEGER NOT NULL,
    response    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_idempotency_created ON idempotency_keys (created_at);

-- ---------- outbox: event Chat -> CMS gui bat dong bo (message.created, user.updated) ----------
CREATE TABLE IF NOT EXISTS webhook_outbox (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id     VARCHAR(64) NOT NULL UNIQUE,
    event_type   VARCHAR(64) NOT NULL,
    payload      JSONB NOT NULL,
    status       VARCHAR(16) NOT NULL DEFAULT 'pending', -- pending | sent | failed
    attempts     INTEGER NOT NULL DEFAULT 0,
    last_error   TEXT NOT NULL DEFAULT '',
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_outbox_pending ON webhook_outbox (next_retry_at) WHERE status = 'pending';
