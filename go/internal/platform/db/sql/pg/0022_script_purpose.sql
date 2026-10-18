-- What a script is for (quest=457): set by hand. '' = unsorted (offered everywhere its sources fit, listed as «Общие»).
ALTER TABLE script ADD COLUMN purpose TEXT NOT NULL DEFAULT '' CHECK (purpose IN ('', 'step', 'note', 'template', 'hook'));
