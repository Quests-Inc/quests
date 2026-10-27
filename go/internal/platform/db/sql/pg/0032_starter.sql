-- Stable identity and two hashes: source content before remapping, installed editable fields after remapping.
-- No FK to the entity: a deletion is remembered and never silently resurrected by a later seed.
CREATE TABLE seed_item (
 workspace_id INTEGER NOT NULL DEFAULT app_workspace() REFERENCES workspace(id) ON DELETE CASCADE,
 key TEXT NOT NULL,
 entity_kind TEXT NOT NULL,
 entity_id INTEGER NOT NULL,
 set_version INTEGER NOT NULL,
 content_hash TEXT NOT NULL,
 applied_hash TEXT NOT NULL,
 removed INTEGER NOT NULL DEFAULT 0 CHECK (removed IN (0,1)),
 PRIMARY KEY (workspace_id,key)
);
ALTER TABLE seed_item ENABLE ROW LEVEL SECURITY;
CREATE POLICY workspace_isolation ON seed_item USING (workspace_id=(SELECT app_workspace())) WITH CHECK (workspace_id=(SELECT app_workspace()));
GRANT SELECT,INSERT,UPDATE,DELETE ON seed_item TO quests_app;
