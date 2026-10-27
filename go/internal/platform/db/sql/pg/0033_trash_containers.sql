-- Containers can be restored without detaching existing quests or templates. Purge detaches them after 30 days.
ALTER TABLE questline ADD COLUMN deleted_at TIMESTAMP, ADD COLUMN deleted_by INTEGER;
ALTER TABLE questtemplate ADD COLUMN deleted_at TIMESTAMP, ADD COLUMN deleted_by INTEGER;
CREATE INDEX ix_questline_deleted_at ON questline(workspace_id,deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX ix_questtemplate_deleted_at ON questtemplate(workspace_id,deleted_at) WHERE deleted_at IS NOT NULL;
CREATE POLICY not_trashed ON questline AS RESTRICTIVE USING (deleted_at IS NULL OR (SELECT app_trash())) WITH CHECK (deleted_at IS NULL OR (SELECT app_trash()));
CREATE POLICY not_trashed ON questtemplate AS RESTRICTIVE USING (deleted_at IS NULL OR (SELECT app_trash())) WITH CHECK (deleted_at IS NULL OR (SELECT app_trash()));
