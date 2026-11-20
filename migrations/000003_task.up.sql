-- Task / nhan task / phieu sua.
-- Thiet ke: "card cua ai hien cua nguoi do" => moi nguoi 1 dong task_assignments.
-- Trang thai tong cua task duoc suy ra tu cac assignment (xem docs/backend.md#module-task-nội-bộ).

-- ---------- teams (to / bo phan) ----------
CREATE TABLE IF NOT EXISTS teams (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code       VARCHAR(64) NOT NULL UNIQUE,
    name       VARCHAR(255) NOT NULL,
    kind       VARCHAR(32) NOT NULL DEFAULT 'design',   -- design | edit | order | other
    enabled    BOOLEAN NOT NULL DEFAULT true,
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS team_members (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       VARCHAR(32) NOT NULL DEFAULT 'member',   -- leader | assistant | member
    active     BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ux_team_member UNIQUE (team_id, user_id),
    CONSTRAINT ck_team_member_role CHECK (role IN ('leader','assistant','member'))
);
CREATE INDEX IF NOT EXISTS ix_team_members_user ON team_members (user_id) WHERE active;

-- ---------- tasks ----------
CREATE SEQUENCE IF NOT EXISTS task_code_seq START 1;

CREATE TABLE IF NOT EXISTS tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code            VARCHAR(32) NOT NULL UNIQUE,                 -- T-000001
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    thread_id       UUID REFERENCES messages(id) ON DELETE SET NULL,  -- card/thread cua yeu cau
    parent_task_id  UUID REFERENCES tasks(id) ON DELETE SET NULL,     -- danh cho "goi Edit" / task con
    submission_id   UUID REFERENCES bot_submissions(id) ON DELETE SET NULL,
    team_id         UUID REFERENCES teams(id) ON DELETE SET NULL,
    title           VARCHAR(500) NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    quantity        INTEGER NOT NULL DEFAULT 1,                   -- so luong yeu cau
    quota           VARCHAR(64) NOT NULL DEFAULT '',              -- dinh muc de xuat
    deadline        TIMESTAMPTZ,
    priority        VARCHAR(16) NOT NULL DEFAULT 'normal',        -- low | normal | high | urgent
    status          VARCHAR(32) NOT NULL DEFAULT 'waiting_accept',
    created_by      UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    completed_at    TIMESTAMPTZ,
    metadata        JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_task_status CHECK (status IN
        ('waiting_accept','in_progress','paused','on_hold','completed','cancelled'))
);
CREATE INDEX IF NOT EXISTS ix_tasks_conv    ON tasks (conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_tasks_status  ON tasks (status);
CREATE INDEX IF NOT EXISTS ix_tasks_team    ON tasks (team_id);
CREATE INDEX IF NOT EXISTS ix_tasks_thread  ON tasks (thread_id);
CREATE INDEX IF NOT EXISTS ix_tasks_creator ON tasks (created_by);

-- ---------- task_assignments (card cua tung nguoi) ----------
CREATE TABLE IF NOT EXISTS task_assignments (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id      UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         VARCHAR(32) NOT NULL DEFAULT 'primary',    -- primary | support | edit
    status       VARCHAR(32) NOT NULL DEFAULT 'pending',
    quota        VARCHAR(64) NOT NULL DEFAULT '',           -- dinh muc nguoi nhan chon luc nhan
    quantity     INTEGER NOT NULL DEFAULT 0,                -- so luong nguoi nhan chon luc nhan
    note         TEXT NOT NULL DEFAULT '',
    assigned_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    assigned_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at  TIMESTAMPTZ,
    paused_at    TIMESTAMPTZ,
    held_at      TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    metadata     JSONB,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_assignment_role CHECK (role IN ('primary','support','edit')),
    CONSTRAINT ck_assignment_status CHECK (status IN
        ('pending','in_progress','paused','on_hold','completed','revoked'))
);
-- 1 nguoi chi co 1 card dang hieu luc tren 1 task.
CREATE UNIQUE INDEX IF NOT EXISTS ux_assignment_active
    ON task_assignments (task_id, user_id) WHERE status <> 'revoked';
CREATE INDEX IF NOT EXISTS ix_assignments_user_status ON task_assignments (user_id, status);
CREATE INDEX IF NOT EXISTS ix_assignments_task ON task_assignments (task_id);

-- ---------- task_revisions (phieu sua) ----------
CREATE SEQUENCE IF NOT EXISTS revision_code_seq START 1;

CREATE TABLE IF NOT EXISTS task_revisions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code          VARCHAR(32) NOT NULL UNIQUE,              -- R-000001
    task_id       UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    assignment_id UUID REFERENCES task_assignments(id) ON DELETE SET NULL,
    reason        TEXT NOT NULL,                            -- nguyen nhan
    revision_type VARCHAR(64) NOT NULL DEFAULT '',          -- loai sua
    quantity      INTEGER NOT NULL DEFAULT 1,               -- so luong
    status        VARCHAR(32) NOT NULL DEFAULT 'open',      -- open | completed | cancelled
    submission_id UUID REFERENCES bot_submissions(id) ON DELETE SET NULL,
    message_id    UUID REFERENCES messages(id) ON DELETE SET NULL,
    created_by    UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    completed_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    completed_at  TIMESTAMPTZ,
    metadata      JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT ck_revision_status CHECK (status IN ('open','completed','cancelled'))
);
CREATE INDEX IF NOT EXISTS ix_revisions_task   ON task_revisions (task_id);
CREATE INDEX IF NOT EXISTS ix_revisions_status ON task_revisions (status);

-- ---------- task_events (lich su / audit) ----------
CREATE TABLE IF NOT EXISTS task_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id       UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    assignment_id UUID REFERENCES task_assignments(id) ON DELETE SET NULL,
    revision_id   UUID REFERENCES task_revisions(id) ON DELETE SET NULL,
    type          VARCHAR(48) NOT NULL,
    actor_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    from_status   VARCHAR(32) NOT NULL DEFAULT '',
    to_status     VARCHAR(32) NOT NULL DEFAULT '',
    message       TEXT NOT NULL DEFAULT '',
    payload       JSONB,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS ix_task_events_task ON task_events (task_id, created_at DESC);

-- ---------- seed to mac dinh ----------
INSERT INTO teams (code, name, kind) VALUES
    ('design', 'To Design', 'design'),
    ('edit',   'To Edit',   'edit'),
    ('order',  'To Order',  'order')
ON CONFLICT (code) DO NOTHING;
