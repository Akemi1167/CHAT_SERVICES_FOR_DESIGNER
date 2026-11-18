-- Sua unique index cua dinh danh CMS: GORM ghi chuoi RONG (khong phai NULL) cho user
-- khong co dinh danh CMS (vd token dev), nen "WHERE col IS NOT NULL" van bat trung ''.
-- Chi coi la co dinh danh khi khac NULL VA khac rong.

DROP INDEX IF EXISTS ux_users_open_id;
DROP INDEX IF EXISTS ux_users_actor_id;

CREATE UNIQUE INDEX ux_users_open_id
    ON users (open_id) WHERE open_id IS NOT NULL AND open_id <> '';
CREATE UNIQUE INDEX ux_users_actor_id
    ON users (actor_id) WHERE actor_id IS NOT NULL AND actor_id <> '';

-- Don du lieu cu neu da lo ghi chuoi rong.
UPDATE users SET open_id = NULL WHERE open_id = '';
UPDATE users SET actor_id = NULL WHERE actor_id = '';
