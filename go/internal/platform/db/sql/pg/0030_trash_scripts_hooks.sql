-- Trash for scripts and hooks, the same way as for quests and notes (0015): deleting one sets deleted_at, a RESTRICTIVE
-- policy hides the row from every ordinary query, and only the trash operations (execution/trash.go) open it with
-- app.trash = '1'. A trashed script keeps its versions; a trashed hook does not fire.
ALTER TABLE script ADD COLUMN deleted_at TIMESTAMP, ADD COLUMN deleted_by INTEGER;
ALTER TABLE hook   ADD COLUMN deleted_at TIMESTAMP, ADD COLUMN deleted_by INTEGER;

CREATE INDEX ix_script_deleted_at ON script (workspace_id, deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX ix_hook_deleted_at   ON hook   (workspace_id, deleted_at) WHERE deleted_at IS NOT NULL;

CREATE POLICY not_trashed ON script AS RESTRICTIVE
    USING (deleted_at IS NULL OR (SELECT app_trash()))
    WITH CHECK (deleted_at IS NULL OR (SELECT app_trash()));
CREATE POLICY not_trashed ON hook AS RESTRICTIVE
    USING (deleted_at IS NULL OR (SELECT app_trash()))
    WITH CHECK (deleted_at IS NULL OR (SELECT app_trash()));
