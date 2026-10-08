-- Workspaces (quest=213): hard data isolation inside one database.
--
-- Every domain table gets workspace_id, defaulting to the session's
-- app.workspace_id so INSERTs need no change, and a row-level-security policy
-- pinning reads and writes to it. The API connects as the login role and drops
-- to quests_app (no BYPASSRLS, not the table owner) with SET ROLE, so the
-- policies bind it; the owner / pg_dump / migrations are unaffected.
-- A session without app.workspace_id sees nothing (fail closed).

CREATE TABLE workspace (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT (now() AT TIME ZONE 'utc')
);
INSERT INTO workspace (id, name) VALUES (1, 'Основное');
SELECT setval(pg_get_serial_sequence('workspace', 'id'), 1);

CREATE TABLE workspace_member (
    workspace_id INTEGER NOT NULL REFERENCES workspace (id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES appuser (id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'owner',
    PRIMARY KEY (workspace_id, user_id)
);
CREATE INDEX ix_workspace_member_user_id ON workspace_member (user_id);
INSERT INTO workspace_member (workspace_id, user_id) SELECT 1, id FROM appuser;

-- Which workspace a login session / API token works in (NULL = the user's first membership).
ALTER TABLE usersession ADD COLUMN workspace_id INTEGER REFERENCES workspace (id) ON DELETE SET NULL;
ALTER TABLE apitoken ADD COLUMN workspace_id INTEGER REFERENCES workspace (id) ON DELETE CASCADE;

CREATE FUNCTION app_workspace() RETURNS integer
    LANGUAGE sql STABLE
    AS $$ SELECT NULLIF(current_setting('app.workspace_id', true), '')::integer $$;

DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'questcategory', 'questline', 'quest', 'queststep', 'questtemplate', 'questtemplatestep',
        'templateemitroll', 'templateemitattempt', 'questchangelog', 'tag', 'quest_tag', 'template_tag',
        'secret', 'note', 'attachment', 'attachment_revision', 'graphdraft_op', 'graphdraft_state',
        'runner', 'runner_job'
    ] LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN workspace_id INTEGER NOT NULL DEFAULT 1 REFERENCES workspace (id) ON DELETE CASCADE', t);
        EXECUTE format('ALTER TABLE %I ALTER COLUMN workspace_id SET DEFAULT app_workspace()', t);
        EXECUTE format('CREATE INDEX %I ON %I (workspace_id)', 'ix_' || t || '_workspace_id', t);
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY workspace_isolation ON %I USING (workspace_id = (SELECT app_workspace())) WITH CHECK (workspace_id = (SELECT app_workspace()))',
            t);
    END LOOP;
END $$;

-- Names that were globally unique are now unique per workspace.
ALTER TABLE questcategory DROP CONSTRAINT questcategory_slug_key;
ALTER TABLE questcategory ADD CONSTRAINT uq_questcategory_workspace_slug UNIQUE (workspace_id, slug);
ALTER TABLE tag DROP CONSTRAINT tag_slug_key;
ALTER TABLE tag ADD CONSTRAINT uq_tag_workspace_slug UNIQUE (workspace_id, slug);
ALTER TABLE runner DROP CONSTRAINT runner_label_key;
ALTER TABLE runner ADD CONSTRAINT uq_runner_workspace_label UNIQUE (workspace_id, label);

-- Runners authenticate with their own token before any workspace is known;
-- these resolve it while bypassing RLS (they run as the owner).
CREATE FUNCTION runner_workspace_by_token(h text) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
    AS $$ SELECT workspace_id FROM runner WHERE token_hash = h $$;
CREATE FUNCTION runner_workspace_by_reg(h text) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public
    AS $$ SELECT workspace_id FROM runner WHERE reg_hash = h $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'quests_app') THEN
        CREATE ROLE quests_app NOLOGIN NOBYPASSRLS;
    END IF;
END $$;
GRANT quests_app TO CURRENT_USER;
GRANT USAGE ON SCHEMA public TO quests_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO quests_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO quests_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO quests_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO quests_app;
