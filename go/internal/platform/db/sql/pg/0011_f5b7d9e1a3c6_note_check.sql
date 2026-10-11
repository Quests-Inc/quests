-- A note can carry a check (quest=370): a command that answers "is this note out of date?", run by the
-- same machinery as a step's check (server in a trusted workspace, or a runner; secrets; interval).
-- The columns mirror queststep's. The command prints 0/1 or {"outdated": 0|1, "reason": "…"}; a broken
-- run is recorded in check_error and leaves `outdated` as it was.
ALTER TABLE note
    ADD COLUMN check_command TEXT,
    ADD COLUMN check_interval_seconds INTEGER,
    ADD COLUMN check_last_run_at TIMESTAMP,
    ADD COLUMN run_on TEXT,
    ADD COLUMN outdated INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN outdated_at TIMESTAMP,
    ADD COLUMN check_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN check_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN check_error_at TIMESTAMP;

-- A runner job can belong to a note.
ALTER TABLE runner_job ADD COLUMN note_id INTEGER;

-- A note has secrets of its own (no inheritance: a note may refer to things, it does not depend on them).
ALTER TABLE secret ADD COLUMN note_id INTEGER REFERENCES note (id) ON DELETE CASCADE;
ALTER TABLE secret DROP CONSTRAINT ck_secret_one_owner;
ALTER TABLE secret ADD CONSTRAINT ck_secret_one_owner CHECK (
    (questline_id IS NOT NULL)::int + (template_id IS NOT NULL)::int + (quest_id IS NOT NULL)::int
    + (step_id IS NOT NULL)::int + (note_id IS NOT NULL)::int = 1);
CREATE UNIQUE INDEX ux_secret_note_id_key ON secret (note_id, key) WHERE note_id IS NOT NULL;
