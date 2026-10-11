-- What the API role may NOT change (quest=360). It owns all domain data through row-level
-- security, but a few columns are the host's: whether a workspace may run commands on the
-- server, who has which password, which schema revision is applied. Those are changed by the
-- owner role only (quests-server admin commands, migrations), so an SQL injection in the API
-- cannot flip `trusted` (a step to running commands on the host) or rewrite a password hash.
REVOKE UPDATE ON workspace FROM quests_app;
GRANT UPDATE (name, deleted_at) ON workspace TO quests_app;      -- rename, soft delete (restore is the admin CLI)

REVOKE UPDATE ON appuser FROM quests_app;
GRANT UPDATE (last_login_at) ON appuser TO quests_app;           -- passwords / activation: admin CLI

REVOKE INSERT, UPDATE, DELETE ON alembic_version FROM quests_app;
