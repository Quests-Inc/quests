-- Stable addresses independent of display names. Deleted workspace slugs remain
-- reserved until purge; repository slugs are unique within their workspace.
ALTER TABLE workspace ADD COLUMN slug TEXT NOT NULL DEFAULT '';
UPDATE workspace SET slug='workspace-' || id;
ALTER TABLE workspace ADD CONSTRAINT workspace_slug_unique UNIQUE(slug);
ALTER TABLE workspace ADD CONSTRAINT workspace_slug_format CHECK(length(slug)<=63 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
ALTER TABLE repository ADD COLUMN slug TEXT NOT NULL DEFAULT '';
UPDATE repository SET slug='repo-' || id;
ALTER TABLE repository ADD CONSTRAINT repository_slug_unique UNIQUE(workspace_id,slug);
ALTER TABLE repository ADD CONSTRAINT repository_slug_format CHECK(length(slug)<=63 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$');
CREATE FUNCTION default_git_slug() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.slug='' THEN
  NEW.slug := CASE TG_TABLE_NAME WHEN 'workspace' THEN 'workspace-' ELSE 'repo-' END || NEW.id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER workspace_default_slug BEFORE INSERT ON workspace FOR EACH ROW EXECUTE FUNCTION default_git_slug();
CREATE TRIGGER repository_default_slug BEFORE INSERT ON repository FOR EACH ROW EXECUTE FUNCTION default_git_slug();

GRANT UPDATE (slug) ON workspace TO quests_app;
