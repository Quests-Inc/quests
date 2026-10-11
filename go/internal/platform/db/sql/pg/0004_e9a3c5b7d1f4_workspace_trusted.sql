-- Server-side command execution is allowed only in trusted workspaces (quest=348).
-- Check commands and emit_pool_command without run_on run `sh -c` inside the API
-- container; for anyone but the operator that would be a shell on the server.
-- The flag is set from the host only: quests-server workspace trust <id>.
ALTER TABLE workspace ADD COLUMN trusted INTEGER NOT NULL DEFAULT 0;
UPDATE workspace SET trusted = 1 WHERE id = 1;
