package hooks

// Dispatch used to run matching hooks from data/hooks.json on the API host
// (script / webhook / socket). Workspace hooks now live in the database and run
// through the evaluator and runners (quest=438); the file path is retired.
func Dispatch(event map[string]any) int {
	_ = event
	return 0
}
