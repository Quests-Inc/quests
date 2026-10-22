-- A value may remember the embedded script and argument that produced it (a roll such as 2к8+2). The value stays a literal:
-- rolling again writes a new one, and formulas over it follow. Both columns are empty for a hand-written value.
ALTER TABLE named_value ADD COLUMN roll_script INTEGER REFERENCES script (id) ON DELETE SET NULL;
ALTER TABLE named_value ADD COLUMN roll_arg TEXT NOT NULL DEFAULT '';
