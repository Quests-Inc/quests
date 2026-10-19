-- A script for embedding in a text (`script=N{…}`, ADR-0003 §6b) has its own purpose.
ALTER TABLE script DROP CONSTRAINT script_purpose_check;
ALTER TABLE script ADD CONSTRAINT script_purpose_check CHECK (purpose IN ('', 'step', 'note', 'template', 'hook', 'embed'));
