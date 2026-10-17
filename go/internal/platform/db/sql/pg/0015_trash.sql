-- Trash: deleting a quest or a note hides it instead of erasing it.
--
-- A row with deleted_at set is invisible to every ordinary query without those queries knowing about it: a
-- RESTRICTIVE row-level-security policy (ANDed with workspace_isolation) lets it through only inside a transaction that
-- has set app.trash = '1' (the trash operations, see journal/trash.go and knowledge/trash.go). Everything that hangs off
-- the row (steps, tags, secrets, attachments, failure inbox, history) stays where it is until the item is purged.

CREATE FUNCTION app_trash() RETURNS boolean
    LANGUAGE sql STABLE
    AS $$ SELECT COALESCE(current_setting('app.trash', true), '') = '1' $$;

ALTER TABLE quest ADD COLUMN deleted_at TIMESTAMP, ADD COLUMN deleted_by INTEGER;
ALTER TABLE note  ADD COLUMN deleted_at TIMESTAMP, ADD COLUMN deleted_by INTEGER,
                  -- children that were lifted to the root when the note went to the trash: restore puts them back
                  ADD COLUMN deleted_children INTEGER[];

CREATE INDEX ix_quest_deleted_at ON quest (workspace_id, deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX ix_note_deleted_at  ON note  (workspace_id, deleted_at) WHERE deleted_at IS NOT NULL;

-- A trashed quest keeps its template period, but must not block the template from emitting that period again.
ALTER TABLE quest DROP CONSTRAINT uq_quest_template_period;
CREATE UNIQUE INDEX uq_quest_template_period ON quest (template_id, period_key) WHERE deleted_at IS NULL;

CREATE POLICY not_trashed ON quest AS RESTRICTIVE
    USING (deleted_at IS NULL OR (SELECT app_trash()))
    WITH CHECK (deleted_at IS NULL OR (SELECT app_trash()));
CREATE POLICY not_trashed ON note AS RESTRICTIVE
    USING (deleted_at IS NULL OR (SELECT app_trash()))
    WITH CHECK (deleted_at IS NULL OR (SELECT app_trash()));
