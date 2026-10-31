-- A workspace's visual identity, shared by its members.
ALTER TABLE workspace ADD COLUMN color TEXT NOT NULL DEFAULT '#9a9a9a';
ALTER TABLE workspace ADD COLUMN icon TEXT NOT NULL DEFAULT 'layers';
GRANT UPDATE (color, icon) ON workspace TO quests_app;
