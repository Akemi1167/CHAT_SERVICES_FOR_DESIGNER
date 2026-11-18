DROP INDEX IF EXISTS ux_users_open_id;
DROP INDEX IF EXISTS ux_users_actor_id;
CREATE UNIQUE INDEX ux_users_open_id  ON users (open_id)  WHERE open_id IS NOT NULL;
CREATE UNIQUE INDEX ux_users_actor_id ON users (actor_id) WHERE actor_id IS NOT NULL;
