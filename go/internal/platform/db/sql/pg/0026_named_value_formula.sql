-- A named value may be a formula over the other values in its scope. Empty means `value` is a literal.
ALTER TABLE named_value ADD COLUMN formula TEXT NOT NULL DEFAULT '';
