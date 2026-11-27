-- Log ung dung ghi vao DB de debug (ngoai stdout / LOG_FILE). Ghi theo lo, bat dong bo;
-- tu xoa dong cu hon LOG_DB_RETENTION. Doc: GET /debug/logs (header X-Debug-Key) hoac `make dblog`.
CREATE TABLE IF NOT EXISTS app_logs (
    id         BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    level      VARCHAR(8)  NOT NULL,          -- DEBUG | INFO | WARN | ERROR
    msg        TEXT        NOT NULL,
    component  VARCHAR(64) NOT NULL DEFAULT '', -- vd service.message, ws, service.cmssync
    request_id VARCHAR(64) NOT NULL DEFAULT '', -- X-Request-ID: gom moi dong log cua 1 request
    method     VARCHAR(8)  NOT NULL DEFAULT '',
    path       VARCHAR(512) NOT NULL DEFAULT '',
    status     INTEGER     NOT NULL DEFAULT 0,
    took_ms    INTEGER     NOT NULL DEFAULT 0,
    attrs      JSONB                            -- moi field con lai cua dong log
);

CREATE INDEX IF NOT EXISTS ix_app_logs_created   ON app_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS ix_app_logs_level     ON app_logs (level, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_app_logs_request   ON app_logs (request_id) WHERE request_id <> '';
CREATE INDEX IF NOT EXISTS ix_app_logs_component ON app_logs (component, created_at DESC) WHERE component <> '';
