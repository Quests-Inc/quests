package mcp

import "os"

// Instructions is what the server tells every client once, at start: principles only, details live in the tool descriptions.
func Instructions() string {
	s := `Quests: a personal journal of quests (tasks with steps), questlines (projects) and notes (a markdown knowledge vault), in workspaces.
- Lists are compact pages {items,total,truncated,next}: take next as cursor for more. Read one thing in full with get_quest / get_step / get_note / get_questline / get_template; long texts are cut at max_chars, continue with offset.
- The active workspace (list_workspaces / switch_workspace) applies to every call; mutation results say which one they hit.
- description fields are markdown, titles are plain text. Link entities as quest=N, step=N, note=N, questline=N, template=N; images in notes only via upload_attachment + ![alt](attachment=N).
- Change steps with add_step / update_step / update_steps (several at once), not by rewriting a quest. Lifecycle (status, pin, title…) is update_quest.
- New notes go into a CATEGORY note (list_note_categories). Private notes are hidden from you on purpose.
- Only ping_user interrupts the user's screen: use it when you are blocked on their answer.`
	if id := os.Getenv("QUESTS_GUIDE_NOTE"); id != "" {
		s += "\n- Fuller agent guide: note=" + id + " (get_note)."
	}
	return s
}

// New wires every tool onto a server that talks through c.
func New(c *Client, version string) *Server {
	s := NewServer("quests", version, Instructions())
	for _, group := range [][]Tool{
		workspaceTools(c), questTools(c), stepTools(c), questlineTools(c), noteTools(c),
		templateTools(c), tagTools(c), secretTools(c), runnerTools(c), graphTools(c), assetTools(c),
	} {
		s.Add(group...)
	}
	return s
}
