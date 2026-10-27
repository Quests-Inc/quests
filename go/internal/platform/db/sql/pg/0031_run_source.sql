-- Runs as a log people read (quest=489). A run says what started it, apart from the entity its sources read:
-- source_kind is step, note, template (a bound script or an implicit check), hook, embed (a text's script=N{…}) or test (the
-- editor's «Запустить на…»), source_id the step, note, template, hook or script. A test run may be dry (its sinks only report
-- what they would write) and may carry a graph that was never saved (graph_ref "inline", the graph in graph_inline).
ALTER TABLE script_run ADD COLUMN source_kind TEXT NOT NULL DEFAULT '';
ALTER TABLE script_run ADD COLUMN source_id INTEGER;
ALTER TABLE script_run ADD COLUMN dry INTEGER NOT NULL DEFAULT 0 CHECK (dry IN (0, 1));
ALTER TABLE script_run ADD COLUMN graph_inline TEXT;

UPDATE script_run SET
    source_kind = CASE WHEN run_key LIKE 'origin:%' THEN 'hook' WHEN run_key LIKE 'embed:%' THEN 'embed' ELSE subject_type END,
    source_id = CASE
        WHEN run_key LIKE 'origin:%' THEN substring(run_key FROM 'hook:([0-9]+)$')::integer
        WHEN run_key LIKE 'embed:%' THEN script_id
        ELSE subject_id END;

CREATE INDEX ix_script_run_source ON script_run (workspace_id, source_kind, source_id, id DESC);
CREATE INDEX ix_script_run_script ON script_run (workspace_id, script_id, id DESC);
