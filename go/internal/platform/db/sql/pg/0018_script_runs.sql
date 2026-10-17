-- A run of a script graph (ADR-0003): the evaluator suspends at a Code node while its job is out, so the run is kept here as
-- a snapshot (node states and the tokens produced so far) until the outcome comes back. graph_ref says which graph it is:
-- "script:<id>:<version>" or "implicit:<name>" (the graph a plain check_command stands for).
CREATE TABLE script_run (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    graph_ref TEXT NOT NULL,
    script_id INTEGER REFERENCES script (id) ON DELETE SET NULL,
    subject_type TEXT NOT NULL,
    subject_id INTEGER NOT NULL,
    state TEXT NOT NULL,
    snapshot TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,
    finished_at TIMESTAMP
);
CREATE INDEX ix_script_run_workspace_id ON script_run (workspace_id);
CREATE INDEX ix_script_run_subject ON script_run (subject_type, subject_id);
ALTER TABLE script_run ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON script_run
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, UPDATE, DELETE ON script_run TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE script_run_id_seq TO quests_app;

-- Which run and which Code node a job is for, and the values that came over links into that node (not secrets: those are
-- resolved when a runner claims the job and are never stored).
ALTER TABLE runner_job ADD COLUMN run_id INTEGER REFERENCES script_run (id) ON DELETE CASCADE;
ALTER TABLE runner_job ADD COLUMN run_node INTEGER;
ALTER TABLE runner_job ADD COLUMN env TEXT;
CREATE INDEX ix_runner_job_run_id ON runner_job (run_id);
