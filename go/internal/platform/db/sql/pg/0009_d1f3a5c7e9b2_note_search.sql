-- Search over notes (quest=354): a generated tsvector (title weighs more than the body; Russian and
-- English stemming side by side, so "квесты/квестов", "deploy/deploys" meet), a GIN index on it, and
-- trigram support for typos in titles. The body is cut at 300k characters: a tsvector may not pass 1 MB.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE note ADD COLUMN search_tsv tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('russian', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('russian', left(coalesce(description, ''), 300000)), 'B') ||
    setweight(to_tsvector('english', left(coalesce(description, ''), 300000)), 'B')
) STORED;
CREATE INDEX ix_note_search_tsv ON note USING GIN (search_tsv);
CREATE INDEX ix_note_title_trgm ON note USING GIN (lower(title) gin_trgm_ops);
