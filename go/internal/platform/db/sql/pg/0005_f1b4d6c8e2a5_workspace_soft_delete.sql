-- Soft deletion of workspaces (quest=347): deleted_at hides a workspace and stops its
-- tokens / runners / sessions at once; the maintenance loop purges it (cascade) after a
-- grace period, `quests-server workspace restore <id>` brings it back before that.
ALTER TABLE workspace ADD COLUMN deleted_at TIMESTAMP;

CREATE OR REPLACE FUNCTION runner_workspace_by_token(h text) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
    AS $$ SELECT r.workspace_id FROM runner r JOIN workspace w ON w.id = r.workspace_id
          WHERE r.token_hash = h AND w.deleted_at IS NULL $$;
CREATE OR REPLACE FUNCTION runner_workspace_by_reg(h text) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
    AS $$ SELECT r.workspace_id FROM runner r JOIN workspace w ON w.id = r.workspace_id
          WHERE r.reg_hash = h AND w.deleted_at IS NULL $$;
