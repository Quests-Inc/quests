-- Scripts (ADR-0003, quest=440): graphs of typed nodes, owner-edited, versioned. A script row carries the current version
-- and what the editor lists (what entities it reads, whether it needs a runner); every saved graph is kept in
-- script_version, because a job that waits for its runner runs the version it was queued with.
CREATE TABLE script (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    subjects TEXT NOT NULL DEFAULT '',
    needs_runner INTEGER NOT NULL DEFAULT 0 CHECK (needs_runner IN (0, 1)),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
CREATE INDEX ix_script_workspace_id ON script (workspace_id);

CREATE TABLE script_version (
    script_id INTEGER NOT NULL REFERENCES script (id) ON DELETE CASCADE,
    version INTEGER NOT NULL,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    graph TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    PRIMARY KEY (script_id, version)
);
CREATE INDEX ix_script_version_workspace_id ON script_version (workspace_id);

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['script', 'script_version'] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY workspace_isolation ON %I USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()))',
            t);
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON %I TO quests_app', t);
    END LOOP;
END $$;
GRANT USAGE, SELECT ON SEQUENCE script_id_seq TO quests_app;
