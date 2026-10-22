-- Named values: readable literals on an entity (a note, a quest, a step, a template, a questline, the workspace).
-- Same owners as a secret, a different contract: the value is returned by the API, it is not sealed, and a note
-- inherits its parents and then the workspace. A check command sees the names in upper case.
CREATE TABLE named_value (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    questline_id INTEGER REFERENCES questline (id) ON DELETE CASCADE,
    template_id INTEGER REFERENCES questtemplate (id) ON DELETE CASCADE,
    quest_id INTEGER REFERENCES quest (id) ON DELETE CASCADE,
    step_id INTEGER REFERENCES queststep (id) ON DELETE CASCADE,
    note_id INTEGER REFERENCES note (id) ON DELETE CASCADE,
    workspace_scope INTEGER NOT NULL DEFAULT 0 CHECK (workspace_scope IN (0, 1)),
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('int', 'float', 'str', 'bool')),
    value TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    CONSTRAINT ck_named_value_one_owner CHECK (
        (questline_id IS NOT NULL)::int + (template_id IS NOT NULL)::int + (quest_id IS NOT NULL)::int
        + (step_id IS NOT NULL)::int + (note_id IS NOT NULL)::int + workspace_scope = 1)
);
CREATE INDEX ix_named_value_workspace_id ON named_value (workspace_id);
CREATE UNIQUE INDEX ux_named_value_questline_name ON named_value (questline_id, name) WHERE questline_id IS NOT NULL;
CREATE UNIQUE INDEX ux_named_value_template_name ON named_value (template_id, name) WHERE template_id IS NOT NULL;
CREATE UNIQUE INDEX ux_named_value_quest_name ON named_value (quest_id, name) WHERE quest_id IS NOT NULL;
CREATE UNIQUE INDEX ux_named_value_step_name ON named_value (step_id, name) WHERE step_id IS NOT NULL;
CREATE UNIQUE INDEX ux_named_value_note_name ON named_value (note_id, name) WHERE note_id IS NOT NULL;
CREATE UNIQUE INDEX ux_named_value_workspace_name ON named_value (workspace_id, name) WHERE workspace_scope = 1;

ALTER TABLE named_value ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON named_value
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON named_value TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE named_value_id_seq TO quests_app;
