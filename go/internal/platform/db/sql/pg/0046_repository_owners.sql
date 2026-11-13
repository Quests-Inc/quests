-- Repositories follow attachment ownership; steps do not own repositories.
DELETE FROM repository_link WHERE owner_type='step';
ALTER TABLE repository_link DROP CONSTRAINT repository_link_owner_type_check;
ALTER TABLE repository_link ADD CHECK(owner_type IN ('quest','questline','note'));
ALTER TABLE repository_link DROP COLUMN path;
