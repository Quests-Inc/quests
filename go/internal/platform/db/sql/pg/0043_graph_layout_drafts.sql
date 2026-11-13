-- Each map layout owns its pending edits and extra roots. Empty scope preserves legacy API callers.
ALTER TABLE graphdraft_op ADD COLUMN layout_key TEXT NOT NULL DEFAULT '';
ALTER TABLE graphdraft_state ADD COLUMN layout_key TEXT NOT NULL DEFAULT '';
CREATE INDEX graphdraft_op_layout ON graphdraft_op(workspace_id,layout_key,id);
CREATE INDEX graphdraft_state_layout ON graphdraft_state(workspace_id,layout_key);
