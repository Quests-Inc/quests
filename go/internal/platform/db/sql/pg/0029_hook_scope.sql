-- Hooks 2.0 (ADR-0004): a hook is no longer tied to one quest. It has an area (the whole workspace, one quest, step or note, or
-- a note and everything under it), limits against storms, and a switch for events of private notes. quest_id stays in step with
-- a quest area for one more release (it carries the cascade on delete and the old API).
ALTER TABLE hook ADD COLUMN scope_kind TEXT NOT NULL DEFAULT 'workspace'
    CHECK (scope_kind IN ('workspace', 'quest', 'step', 'note', 'note_tree'));
ALTER TABLE hook ADD COLUMN scope_id INTEGER;
ALTER TABLE hook ADD COLUMN cooldown_sec INTEGER NOT NULL DEFAULT 5 CHECK (cooldown_sec >= 0 AND cooldown_sec <= 86400);
ALTER TABLE hook ADD COLUMN max_per_hour INTEGER NOT NULL DEFAULT 120 CHECK (max_per_hour >= 1 AND max_per_hour <= 10000);
ALTER TABLE hook ADD COLUMN include_private INTEGER NOT NULL DEFAULT 0 CHECK (include_private IN (0, 1));
ALTER TABLE hook ADD COLUMN disabled_reason TEXT NOT NULL DEFAULT '';
UPDATE hook SET scope_kind = 'quest', scope_id = quest_id WHERE quest_id IS NOT NULL;
