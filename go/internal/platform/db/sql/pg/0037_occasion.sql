-- A calendar occasion (UI «Событие»): a start and an end, not a quest.
-- Code name is occasion so it does not collide with hook events.
CREATE TABLE occasion (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    starts_at TIMESTAMP NOT NULL,
    ends_at TIMESTAMP NOT NULL,
    all_day INTEGER NOT NULL DEFAULT 0 CHECK (all_day IN (0, 1)),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CHECK (ends_at >= starts_at)
);
CREATE INDEX ix_occasion_workspace_id ON occasion (workspace_id);

ALTER TABLE occasion ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON occasion
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON occasion TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE occasion_id_seq TO quests_app;

-- A quest deadline may be derived from an occasion. deadline_at stays the resolved instant.
ALTER TABLE quest ADD COLUMN occasion_id INTEGER REFERENCES occasion (id) ON DELETE SET NULL;
ALTER TABLE quest ADD COLUMN occasion_edge TEXT;
ALTER TABLE quest ADD COLUMN occasion_dir TEXT;
ALTER TABLE quest ADD COLUMN occasion_offset_seconds INTEGER;
CREATE INDEX ix_quest_occasion_id ON quest (occasion_id) WHERE occasion_id IS NOT NULL;
