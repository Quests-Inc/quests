-- Workspace event hooks (quest=438, ADR-0003): an event trigger plus a script from the library.
-- Delivery is through runners; the old host-admin file (data/hooks.json) is retired separately.
CREATE TABLE hook (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    events TEXT NOT NULL,
    events_raw TEXT NOT NULL DEFAULT '[]',
    script_id INTEGER NOT NULL REFERENCES script (id) ON DELETE RESTRICT,
    quest_id INTEGER REFERENCES quest (id) ON DELETE CASCADE,
    run_on TEXT NOT NULL DEFAULT '',
    timeout_sec INTEGER NOT NULL DEFAULT 30 CHECK (timeout_sec >= 1 AND timeout_sec <= 600),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
CREATE INDEX ix_hook_workspace_id ON hook (workspace_id);
CREATE INDEX ix_hook_script_id ON hook (script_id);
CREATE INDEX ix_hook_quest_id ON hook (quest_id) WHERE quest_id IS NOT NULL;

ALTER TABLE hook ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON hook
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON hook TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE hook_id_seq TO quests_app;
