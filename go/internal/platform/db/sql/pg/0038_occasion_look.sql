-- Per-occasion colour, built-in icon, and an optional banner cropped onto the calendar chip.
ALTER TABLE occasion ADD COLUMN color TEXT NOT NULL DEFAULT '#5a8a9a';
ALTER TABLE occasion ADD COLUMN icon TEXT NOT NULL DEFAULT 'calendar';
ALTER TABLE occasion ADD COLUMN banner TEXT;
