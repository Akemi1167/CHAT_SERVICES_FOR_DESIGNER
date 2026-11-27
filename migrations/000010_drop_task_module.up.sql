-- Go module task noi bo cua Chat: CMS so huu don/task, Chat chi luu card (messages.card).
-- Code da go cung luc (service/task, REST /api/v1/tasks|teams|revisions, gRPC TaskService).
-- XOA HAN du lieu 6 bang duoi day. Module chi chay khi TASK_ENGINE_ENABLED=true (mac dinh tat).
DROP TABLE IF EXISTS task_events;
DROP TABLE IF EXISTS task_revisions;
DROP TABLE IF EXISTS task_assignments;
DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS team_members;
DROP TABLE IF EXISTS teams;
DROP SEQUENCE IF EXISTS revision_code_seq;
DROP SEQUENCE IF EXISTS task_code_seq;
