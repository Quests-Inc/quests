package mcp

// Instructions is what the server tells every client once, at start: principles only, details live in the tool descriptions.
func Instructions() string {
	return `Quests: a personal journal of quests (tasks with steps), questlines (projects) and notes (a markdown knowledge vault), in workspaces.
- Lists are compact pages {items,total,truncated,next}: take next as cursor for more. Read one thing in full with get_quest / get_step / get_note / get_questline / get_template; long texts are cut at max_chars, continue with offset.
- The active workspace (list_workspaces / switch_workspace) applies to every call; mutation results say which one they hit.
- description fields are markdown, titles are plain text. Link entities as quest=N, step=N, note=N, questline=N, template=N; images in notes only via upload_attachment + ![alt](attachment=N).
- Every referenced entity must have its own full kind=ID token, even in a list or range: write quest=438, quest=440 or note=217, note=218, note=219. Never share a prefix across IDs: quest=438/440, quest=438, 440 and note=217…219 are not supported.
- Change steps with add_step / update_step / update_steps (several at once), not by rewriting a quest. Lifecycle (status, pin, title…) is update_quest.
- Lay out what you create: before create_quest look at list_questlines (pass questline, or "none" on purpose) and list_tags; new notes go into a CATEGORY note (list_note_categories). Never leave work unsorted by default.
- Scripts (checks, template emit): the script tool; a script is a graph of typed nodes — docs/adr/0003-script-graphs.md and docs/scripts.md (rules: docs/automation.md).
- Private notes are hidden from you on purpose.
- Only ping_user interrupts the user's screen: use it when you are blocked on their answer.`
}

// New wires every tool onto a server that talks through c.
func New(c *Client, version string) *Server {
	s := NewServer("quests", version, Instructions())
	for _, group := range [][]Tool{
		workspaceTools(c), questTools(c), stepTools(c), questlineTools(c), noteTools(c),
		templateTools(c), scriptTools(c), hookTools(c), tagTools(c), secretTools(c), valueTools(c), runnerTools(c), graphTools(c), assetTools(c),
	} {
		s.Add(group...)
	}
	return s
}
