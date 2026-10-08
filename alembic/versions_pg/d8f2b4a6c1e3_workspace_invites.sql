-- One-time invitation links to a workspace (quest=213). Only the token hash is stored.
CREATE TABLE workspace_invite (
    id SERIAL PRIMARY KEY,
    workspace_id INTEGER NOT NULL REFERENCES workspace (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL DEFAULT 'member',
    created_by INTEGER REFERENCES appuser (id) ON DELETE SET NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    used_at TIMESTAMP,
    used_by INTEGER REFERENCES appuser (id) ON DELETE SET NULL
);
CREATE INDEX ix_workspace_invite_workspace_id ON workspace_invite (workspace_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON workspace_invite TO quests_app;
GRANT USAGE, SELECT ON SEQUENCE workspace_invite_id_seq TO quests_app;
