-- Reading signals per note for ranking (quest=353): aggregates only, never a log of who read what when.
-- `score` is a decaying sum (halves every 30 days, applied lazily from score_at); human opens weigh
-- more than an agent's, an explicit "useful / not useful" most. Per workspace, like everything else.
CREATE TABLE note_stat (
    note_id INTEGER PRIMARY KEY REFERENCES note (id) ON DELETE CASCADE,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    score_at TIMESTAMP,
    opens_human INTEGER NOT NULL DEFAULT 0,
    opens_agent INTEGER NOT NULL DEFAULT 0,
    shown INTEGER NOT NULL DEFAULT 0,
    last_human_at TIMESTAMP,
    last_agent_at TIMESTAMP,
    rated_up INTEGER NOT NULL DEFAULT 0,
    rated_down INTEGER NOT NULL DEFAULT 0,
    manual INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ix_note_stat_workspace_id ON note_stat (workspace_id);
ALTER TABLE note_stat ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON note_stat
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON note_stat TO quests_app;
