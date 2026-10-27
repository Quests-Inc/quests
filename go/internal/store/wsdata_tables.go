package store

type polyFK struct {
	TypeCol, IDCol string
}

// TableSpec describes how one table travels. Tables are listed in insertion order.
type TableSpec struct {
	Name   string
	HasID  bool
	FK     map[string]string // column → referenced table
	Poly   []polyFK          // (type column, id column) pairs naming their target table by type
	SelfFK string            // column referencing the same table, fixed up in a second pass
	Text   []string          // columns scanned for kind=ID references
	// Secret tables are exported only on request.
	Secret bool
	// Where drops the rows that belong to something in the trash. The trash policy hides a trashed quest or note
	// itself, but not the rows hanging off it, and those must not travel with the archive.
	Where string
}

const (
	liveQuest  = `(SELECT id FROM quest)`
	liveNote   = `(SELECT id FROM note)`
	liveStep   = `(SELECT id FROM queststep WHERE quest_id IN ` + liveQuest + `)`
	liveOwner  = `(owner_type NOT IN ('quest', 'note', 'questline') OR owner_id IN (SELECT id FROM quest WHERE owner_type = 'quest' UNION ALL SELECT id FROM note WHERE owner_type = 'note' UNION ALL SELECT id FROM questline WHERE owner_type = 'questline'))`
	liveAttach = `(SELECT id FROM attachment WHERE ` + liveOwner + `)`
)

// Kind names in kind=ID tokens and entity_type / owner_type columns → table.
var kindTable = map[string]string{
	"note_tree": "note", "quest": "quest", "questline": "questline", "note": "note", "step": "queststep",
	"script": "script", "hook": "hook", "attachment": "attachment", "template": "questtemplate", "tag": "tag", "category": "questcategory",
}

var ExportTables = []TableSpec{
	{Name: "script", HasID: true, Text: []string{"title", "description"}},
	{Name: "script_version", Where: `script_id IN (SELECT id FROM script)`, FK: map[string]string{"script_id": "script"}},
	{Name: "custom_node", HasID: true, Text: []string{"title", "description"}},
	{Name: "questcategory", HasID: true},
	{Name: "questline", HasID: true, FK: map[string]string{"category_id": "questcategory"}, Text: []string{"title", "description"}},
	{Name: "questtemplate", HasID: true, FK: map[string]string{"category_id": "questcategory", "questline_id": "questline"}, Text: []string{"title", "description"}},
	{Name: "questtemplatestep", HasID: true, Where: `template_id IN (SELECT id FROM questtemplate)`, FK: map[string]string{"template_id": "questtemplate", "script_id": "script"}, Text: []string{"title", "description", "check_command"}},
	{Name: "quest", HasID: true, FK: map[string]string{"category_id": "questcategory", "questline_id": "questline", "template_id": "questtemplate"}, Text: []string{"title", "description", "source"}},
	{Name: "queststep", HasID: true, Where: `quest_id IN ` + liveQuest, FK: map[string]string{"quest_id": "quest"}, Text: []string{"title", "description", "check_command"}},
	{Name: "tag", HasID: true},
	{Name: "quest_tag", Where: `quest_id IN ` + liveQuest, FK: map[string]string{"quest_id": "quest", "tag_id": "tag"}},
	{Name: "template_tag", Where: `template_id IN (SELECT id FROM questtemplate)`, FK: map[string]string{"template_id": "questtemplate", "tag_id": "tag"}},
	{Name: "note", HasID: true, SelfFK: "parent_id", Text: []string{"title", "description"}},
	{Name: "attachment", HasID: true, Where: liveOwner, Poly: []polyFK{{"owner_type", "owner_id"}}, Text: []string{"comment"}},
	{Name: "attachment_revision", HasID: true, Where: `attachment_id IN ` + liveAttach, FK: map[string]string{"attachment_id": "attachment"}, Text: []string{"comment"}},
	{Name: "templateemitroll", HasID: true, Where: `template_id IN (SELECT id FROM questtemplate)`, FK: map[string]string{"template_id": "questtemplate"}, Text: []string{"picked_refs"}},
	{Name: "questchangelog", HasID: true, Where: `quest_id IS NULL OR quest_id IN ` + liveQuest, FK: map[string]string{"quest_id": "quest"}, Poly: []polyFK{{"entity_type", "entity_id"}, {"parent_type", "parent_id"}}, Text: []string{"title", "detail", "comment"}},
	{Name: "secret", HasID: true, Where: `(questline_id IS NULL OR questline_id IN (SELECT id FROM questline)) AND (template_id IS NULL OR template_id IN (SELECT id FROM questtemplate)) AND (quest_id IS NULL OR quest_id IN ` + liveQuest + `) AND (note_id IS NULL OR note_id IN ` + liveNote + `) AND (step_id IS NULL OR step_id IN ` + liveStep + `)`, FK: map[string]string{"questline_id": "questline", "template_id": "questtemplate", "quest_id": "quest", "step_id": "queststep", "note_id": "note"}, Secret: true},
	{Name: "named_value", HasID: true, Where: `(questline_id IS NULL OR questline_id IN (SELECT id FROM questline)) AND (template_id IS NULL OR template_id IN (SELECT id FROM questtemplate)) AND (quest_id IS NULL OR quest_id IN ` + liveQuest + `) AND (note_id IS NULL OR note_id IN ` + liveNote + `) AND (step_id IS NULL OR step_id IN ` + liveStep + `)`, FK: map[string]string{"questline_id": "questline", "template_id": "questtemplate", "quest_id": "quest", "step_id": "queststep", "note_id": "note", "roll_script": "script"}},
	{Name: "hook", HasID: true, FK: map[string]string{"script_id": "script", "quest_id": "quest"}, Poly: []polyFK{{"scope_kind", "scope_id"}}},
	{Name: "script_binding", HasID: true, Where: `script_id IN (SELECT id FROM script) AND ((subject_type='note' AND subject_id IN (SELECT id FROM note)) OR (subject_type='step' AND subject_id IN ` + liveStep + `) OR (subject_type='template' AND subject_id IN (SELECT id FROM questtemplate)))`, FK: map[string]string{"script_id": "script"}, Poly: []polyFK{{"subject_type", "subject_id"}}},
}
