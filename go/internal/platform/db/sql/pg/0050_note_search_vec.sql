-- Note search vector: the note-search body (title, description and own variable
-- names+values — the same text the matcher used to tokenize per row per query)
-- computed once on write, so the entity-search matcher narrows candidates by the
-- GIN index instead of scanning and tokenizing every note on every request.
-- formula is deliberately excluded (only value counts, see note_search_vars.go),
-- and the text is capped at 300000 runes worth of chars like the inline matcher's
-- left(body, 300000).

ALTER TABLE note ADD COLUMN search_vec tsvector;

CREATE FUNCTION note_search_text (p_note_id INTEGER) RETURNS TEXT
LANGUAGE sql STABLE
SET search_path = pg_catalog, public
AS $$
    SELECT left(
        n.title || ' ' || n.description || ' ' || coalesce(
            (SELECT string_agg(nv.name || ' ' || nv.value, ' ')
             FROM named_value nv WHERE nv.note_id = n.id), ''),
        300000)
    FROM note n WHERE n.id = p_note_id
$$;

-- Trigger bodies: EXECUTE FUNCTION arguments must be literals, so the row id
-- reaches the refresh through dedicated functions per subject table.
CREATE FUNCTION note_search_vec_refresh_note () RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE note SET search_vec = to_tsvector('russian', note_search_text(NEW.id))
    WHERE id = NEW.id;
    RETURN NULL;
END $$;

CREATE FUNCTION note_search_vec_refresh_named_value () RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    v_note_id INTEGER;
BEGIN
    IF TG_OP = 'DELETE' THEN
        v_note_id := OLD.note_id;
    ELSE
        IF TG_OP = 'UPDATE' AND OLD.note_id IS DISTINCT FROM NEW.note_id THEN
            UPDATE note SET search_vec = to_tsvector('russian', note_search_text(OLD.note_id))
            WHERE id = OLD.note_id;
        END IF;
        v_note_id := NEW.note_id;
    END IF;
    UPDATE note SET search_vec = to_tsvector('russian', note_search_text(v_note_id))
    WHERE id = v_note_id;
    RETURN NULL;
END $$;

CREATE TRIGGER note_search_vec_note
AFTER INSERT OR UPDATE OF title, description ON note
FOR EACH ROW EXECUTE FUNCTION note_search_vec_refresh_note();

CREATE TRIGGER note_search_vec_named_value_write
AFTER INSERT OR UPDATE OF name, value, note_id ON named_value
FOR EACH ROW EXECUTE FUNCTION note_search_vec_refresh_named_value();

CREATE TRIGGER note_search_vec_named_value_delete
AFTER DELETE ON named_value
FOR EACH ROW EXECUTE FUNCTION note_search_vec_refresh_named_value();

-- Backfill everything that exists, then let queries use the index.
UPDATE note SET search_vec = to_tsvector('russian', note_search_text(id));
CREATE INDEX note_search_vec_gin ON note USING GIN (search_vec);
ANALYZE note;
