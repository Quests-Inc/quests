CREATE TABLE repository (
 id SERIAL PRIMARY KEY,
 workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace(id) ON DELETE CASCADE,
 title TEXT NOT NULL,
 kind TEXT NOT NULL CHECK(kind IN ('local','github','gitlab')),
 url TEXT NOT NULL DEFAULT '',
 branch TEXT NOT NULL DEFAULT 'main',
 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX repository_workspace ON repository(workspace_id);
ALTER TABLE repository ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON repository USING (workspace_id=(SELECT app_workspace())) WITH CHECK(workspace_id=(SELECT app_workspace()));
CREATE TABLE repository_token (
 repository_id INTEGER PRIMARY KEY REFERENCES repository(id) ON DELETE CASCADE,
 workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace(id) ON DELETE CASCADE,
 value TEXT NOT NULL
);
ALTER TABLE repository_token ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON repository_token USING (workspace_id=(SELECT app_workspace())) WITH CHECK(workspace_id=(SELECT app_workspace()));
CREATE TABLE repository_link (
 repository_id INTEGER NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
 workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace(id) ON DELETE CASCADE,
 owner_type TEXT NOT NULL CHECK(owner_type IN ('quest','questline','step')),
 owner_id INTEGER NOT NULL,
 path TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(repository_id,owner_type,owner_id)
);
ALTER TABLE repository_link ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON repository_link USING (workspace_id=(SELECT app_workspace())) WITH CHECK(workspace_id=(SELECT app_workspace()));
