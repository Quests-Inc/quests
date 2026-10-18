-- Custom nodes (quest=458): a node the owner bakes from other nodes or writes as code. The definition (inner graph, inputs,
-- outputs, editable fields) is JSON; a script uses the node as `custom.<id>` and it is expanded into plain nodes before a run.
CREATE TABLE custom_node (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 1,
    definition TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
CREATE INDEX ix_custom_node_workspace_id ON custom_node (workspace_id);

ALTER TABLE custom_node ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON custom_node
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON custom_node TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE custom_node_id_seq TO quests_app;
