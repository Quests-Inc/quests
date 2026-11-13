-- Calendar-month dates and anchored custom day cycles; existing schedules are unchanged.
ALTER TABLE questtemplate ADD COLUMN monthdays TEXT NOT NULL DEFAULT '1';
ALTER TABLE questtemplate ADD COLUMN cycle_length INTEGER NOT NULL DEFAULT 4;
ALTER TABLE questtemplate ADD COLUMN cycle_start TEXT NOT NULL DEFAULT '';
ALTER TABLE questtemplate ADD COLUMN cycle_days TEXT NOT NULL DEFAULT '1,2';
