-- A secret owned by the workspace itself (ADR-0003): the root of the inheritance chain, seen by every command inside the
-- workspace (steps, quests, templates, questlines, notes) unless something nearer sets the same name. Owner columns stay
-- as they are; the row says it belongs to the workspace by workspace_scope, and workspace_id (already there) says which.
ALTER TABLE secret ADD COLUMN workspace_scope INTEGER NOT NULL DEFAULT 0 CHECK (workspace_scope IN (0, 1));
ALTER TABLE secret DROP CONSTRAINT ck_secret_one_owner;
ALTER TABLE secret ADD CONSTRAINT ck_secret_one_owner CHECK (
    (questline_id IS NOT NULL)::int + (template_id IS NOT NULL)::int + (quest_id IS NOT NULL)::int
    + (step_id IS NOT NULL)::int + (note_id IS NOT NULL)::int + workspace_scope = 1);
CREATE UNIQUE INDEX ux_secret_workspace_key ON secret (workspace_id, "key") WHERE workspace_scope = 1;
