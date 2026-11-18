-- Schema khoi tao cho chat service.
-- Quy uoc: moi thay doi schema = 1 cap file migration moi (khong sua file da chay).

CREATE EXTENSION IF NOT EXISTS "pgcrypto";   -- gen_random_uuid()
CREATE EXTENSION IF NOT EXISTS "pg_trgm";    -- tang toc tim kiem ILIKE

-- ---------- users ----------
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id   VARCHAR(128) NOT NULL UNIQUE,
    display_name  VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url    VARCHAR(1024) NOT NULL DEFAULT '',
    email         VARCHAR(255) NOT NULL DEFAULT '',
    role          VARCHAR(64)  NOT NULL DEFAULT '',
    presence      VARCHAR(32)  NOT NULL DEFAULT 'offline',
    last_seen_at  TIMESTAMPTZ,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_users_email ON users (email);
CREATE INDEX IF NOT EXISTS ix_users_role  ON users (role);

-- ---------- conversations ----------
CREATE TABLE IF NOT EXISTS conversations (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type             VARCHAR(32) NOT NULL,             -- direct | group
    name             VARCHAR(255) NOT NULL DEFAULT '',
    avatar_url       VARCHAR(1024) NOT NULL DEFAULT '',
    direct_key       VARCHAR(64),                      -- NULL voi group
    created_by       UUID NOT NULL,
    last_message_id  UUID,
    last_message_at  TIMESTAMPTZ,
    metadata         JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_conversations_type CHECK (type IN ('direct','group'))
);
-- Dam bao 2 nguoi chi co 1 cuoc chat rieng.
CREATE UNIQUE INDEX IF NOT EXISTS ux_conversations_direct_key
    ON conversations (direct_key) WHERE direct_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_conversations_type ON conversations (type);
CREATE INDEX IF NOT EXISTS ix_conversations_last_message_at ON conversations (last_message_at DESC);

-- ---------- conversation_members ----------
CREATE TABLE IF NOT EXISTS conversation_members (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id      UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role                 VARCHAR(32) NOT NULL DEFAULT 'member',
    last_read_message_id UUID,
    last_read_at         TIMESTAMPTZ,
    muted                BOOLEAN NOT NULL DEFAULT false,
    joined_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at              TIMESTAMPTZ,
    CONSTRAINT ux_conv_member UNIQUE (conversation_id, user_id),
    CONSTRAINT ck_member_role CHECK (role IN ('owner','admin','member'))
);
CREATE INDEX IF NOT EXISTS ix_members_user ON conversation_members (user_id) WHERE left_at IS NULL;

-- ---------- messages ----------
CREATE TABLE IF NOT EXISTS messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       UUID REFERENCES users(id) ON DELETE SET NULL,  -- NULL = bot/system
    sender_type     VARCHAR(16) NOT NULL DEFAULT 'user',
    type            VARCHAR(32) NOT NULL DEFAULT 'text',
    content         TEXT NOT NULL DEFAULT '',
    thread_id       UUID REFERENCES messages(id) ON DELETE SET NULL,
    reply_to_id     UUID REFERENCES messages(id) ON DELETE SET NULL,
    metadata        JSONB,
    client_msg_id   VARCHAR(64) NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT ck_messages_sender_type CHECK (sender_type IN ('user','bot','system'))
);
-- Timeline: lay tin nhan moi nhat cua 1 hoi thoai.
CREATE INDEX IF NOT EXISTS ix_msg_conv_created
    ON messages (conversation_id, created_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS ix_msg_thread ON messages (thread_id) WHERE thread_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS ix_msg_sender ON messages (sender_id);
CREATE INDEX IF NOT EXISTS ix_msg_deleted_at ON messages (deleted_at);
-- Idempotency: FE retry cung client_msg_id se khong tao tin nhan trung.
CREATE UNIQUE INDEX IF NOT EXISTS ux_msg_client
    ON messages (conversation_id, client_msg_id) WHERE client_msg_id <> '';
-- Tim kiem noi dung (ILIKE '%...%').
CREATE INDEX IF NOT EXISTS ix_msg_content_trgm ON messages USING gin (content gin_trgm_ops);

-- ---------- attachments ----------
CREATE TABLE IF NOT EXISTS attachments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    kind        VARCHAR(32) NOT NULL,
    url         VARCHAR(2048) NOT NULL,
    name        VARCHAR(512) NOT NULL DEFAULT '',
    mime_type   VARCHAR(128) NOT NULL DEFAULT '',
    size_bytes  BIGINT NOT NULL DEFAULT 0,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_attachments_message ON attachments (message_id);

-- ---------- message_receipts ----------
CREATE TABLE IF NOT EXISTS message_receipts (
    message_id UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id)
);

-- ---------- bots ----------
CREATE TABLE IF NOT EXISTS bots (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        VARCHAR(64) NOT NULL UNIQUE,
    name        VARCHAR(128) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bot_templates (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id      UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    code        VARCHAR(64) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    description VARCHAR(1024) NOT NULL DEFAULT '',
    schema      JSONB,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ux_bot_template_code UNIQUE (bot_id, code)
);

-- Moi lan "@bot ... gui mau" duoc luu o day (phase 1: chi luu).
CREATE TABLE IF NOT EXISTS bot_submissions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id          UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    bot_slug        VARCHAR(64) NOT NULL,
    template_id     UUID REFERENCES bot_templates(id) ON DELETE SET NULL,
    template_code   VARCHAR(64) NOT NULL DEFAULT '',
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    message_id      UUID REFERENCES messages(id) ON DELETE SET NULL,
    thread_id       UUID REFERENCES messages(id) ON DELETE SET NULL,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    raw_text        TEXT NOT NULL DEFAULT '',
    payload         JSONB,
    status          VARCHAR(32) NOT NULL DEFAULT 'stored',
    error           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_submission_status CHECK (status IN ('stored','processing','done','failed'))
);
CREATE INDEX IF NOT EXISTS ix_submissions_bot    ON bot_submissions (bot_id);
CREATE INDEX IF NOT EXISTS ix_submissions_conv   ON bot_submissions (conversation_id);
CREATE INDEX IF NOT EXISTS ix_submissions_user   ON bot_submissions (user_id);
CREATE INDEX IF NOT EXISTS ix_submissions_status ON bot_submissions (status);
CREATE INDEX IF NOT EXISTS ix_submissions_payload ON bot_submissions USING gin (payload);
