-- Deadline source, urgency window and expiry are independent.
-- Preserve the former behaviour of every existing quest/template.
ALTER TABLE quest ADD COLUMN expire_on_deadline INTEGER NOT NULL DEFAULT 0 CHECK (expire_on_deadline IN (0,1));
UPDATE quest SET expire_on_deadline=1 WHERE duration_seconds>0;
ALTER TABLE questtemplate ADD COLUMN expire_on_deadline INTEGER NOT NULL DEFAULT 0 CHECK (expire_on_deadline IN (0,1));
UPDATE questtemplate SET expire_on_deadline=1 WHERE duration_seconds>0 OR (duration_seconds IS NULL AND deadline_time IS NOT NULL);
