-- A step that follows another quest (quest=298): the link is a hook with an area of the followed quest whose script writes the
-- quest's progress onto the step. The hook knows which step it feeds, and goes away with that step.
ALTER TABLE hook ADD COLUMN mirror_step_id INTEGER REFERENCES queststep(id) ON DELETE CASCADE;
CREATE UNIQUE INDEX ux_hook_mirror_step ON hook(mirror_step_id) WHERE mirror_step_id IS NOT NULL;
