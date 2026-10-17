-- Which script a step or a note runs (ADR-0003): the binding of a script to its subject. The step's or note's own columns
-- (runner, mode, interval) stay where they are; a bound script takes the place of the check_command. A script that is bound
-- cannot be deleted (RESTRICT), and a binding whose subject is gone is swept by the scheduler (no foreign key can say that
-- for two kinds of subject).
CREATE TABLE script_binding (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('step', 'note')),
    subject_id INTEGER NOT NULL,
    script_id INTEGER NOT NULL REFERENCES script (id) ON DELETE RESTRICT,
    created_at TIMESTAMP NOT NULL,
    UNIQUE (workspace_id, subject_type, subject_id)
);
CREATE INDEX ix_script_binding_workspace_id ON script_binding (workspace_id);
CREATE INDEX ix_script_binding_script_id ON script_binding (script_id);
ALTER TABLE script_binding ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON script_binding
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON script_binding TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE script_binding_id_seq TO quests_app;
