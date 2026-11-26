-- The notes list (ListNotes) orders by pinned DESC, sort_order, id inside the
-- RLS-filtered workspace; the composite index serves that order without an
-- in-memory sort once the vault outgrows cheap sorting.
CREATE INDEX ix_note_list_order ON note (workspace_id, pinned DESC, sort_order, id);
ANALYZE note;
