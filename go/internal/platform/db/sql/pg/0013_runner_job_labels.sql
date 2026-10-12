-- A command may name several runners (`run_on` = "a,b,c"): it goes to the first that is online and falls
-- to the next when that one does not answer. The job keeps the whole list; `runner_label` is the one it
-- currently waits for.
ALTER TABLE runner_job ADD COLUMN runner_labels TEXT;
