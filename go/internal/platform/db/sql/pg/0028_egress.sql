-- Outgoing HTTP of scripts (ADR-0004). The administrator of the server decides what a workspace may do: egress_grant is written
-- only by the admin CLI (the database owner); the API role reads its own row and can neither add nor change it.
CREATE TABLE egress_grant (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL UNIQUE REFERENCES workspace (id) ON DELETE CASCADE,
    enabled INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP NOT NULL
);
ALTER TABLE egress_grant ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON egress_grant USING (workspace_id = (SELECT app_workspace()));
-- New tables get full DML for the API role by default privileges (0002); this one is read-only for it.
REVOKE INSERT, UPDATE, DELETE ON egress_grant FROM quests_app;
REVOKE USAGE ON SEQUENCE egress_grant_id_seq FROM quests_app;

-- One line per attempt, refused ones included. Kept for 30 days.
CREATE TABLE egress_log (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace (id) ON DELETE CASCADE,
    at TIMESTAMP NOT NULL,
    script_id INTEGER,
    host TEXT NOT NULL,
    method TEXT NOT NULL,
    status INTEGER NOT NULL DEFAULT 0,
    bytes BIGINT NOT NULL DEFAULT 0,
    millis INTEGER NOT NULL DEFAULT 0,
    result TEXT NOT NULL
);
CREATE INDEX ix_egress_log_workspace_id ON egress_log (workspace_id);
CREATE INDEX ix_egress_log_at ON egress_log (at);
ALTER TABLE egress_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON egress_log
    USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()));
GRANT SELECT, INSERT, DELETE ON egress_log TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE egress_log_id_seq TO quests_app;
