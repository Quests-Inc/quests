-- Saved layouts of the relations map: what the map shows (centre, depth, filters, open links) and where its cards stand.
-- Personal: a layout belongs to the user who saved it, inside one workspace.
CREATE TABLE graph_layout (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    user_id INTEGER REFERENCES appuser (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    state TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
CREATE INDEX ix_graph_layout_owner ON graph_layout (workspace_id, user_id);

ALTER TABLE graph_layout ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON graph_layout
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON graph_layout TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE graph_layout_id_seq TO quests_app;
