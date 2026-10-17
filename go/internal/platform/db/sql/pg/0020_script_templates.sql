-- Scripts on templates (ADR-0003): a template's emit command and the steps it prints can be a script too.
-- The emit script is bound like a step's or a note's (subject 'template'); a template step carries its own script, which the
-- quest it makes is bound to. A run that is a template's emit is found again by its key (template, period, script version) and
-- hands its result to the emit tick through script_run.result.
ALTER TABLE script_binding DROP CONSTRAINT script_binding_subject_type_check;
ALTER TABLE script_binding ADD CONSTRAINT script_binding_subject_type_check CHECK (subject_type IN ('step', 'note', 'template'));

ALTER TABLE questtemplatestep ADD COLUMN script_id INTEGER REFERENCES script (id) ON DELETE RESTRICT;
CREATE INDEX ix_questtemplatestep_script_id ON questtemplatestep (script_id);

ALTER TABLE script_run ADD COLUMN run_key TEXT;
ALTER TABLE script_run ADD COLUMN result TEXT;
CREATE INDEX ix_script_run_key ON script_run (run_key) WHERE run_key IS NOT NULL;
