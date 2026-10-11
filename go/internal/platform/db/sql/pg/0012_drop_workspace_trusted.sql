-- Commands run on runners only (quests-runner, on any device, the server's own host included): the
-- server no longer executes check/emit commands itself, so workspaces are no longer "trusted" or not.
-- Commands stored without a runner stay in the table but never run until a runner is chosen for them.
ALTER TABLE workspace DROP COLUMN trusted;
