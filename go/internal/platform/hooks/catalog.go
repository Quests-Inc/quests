package hooks

// EventInfo describes one kind of event a hook may wait for: which family of entity it is about, what it is called for a
// person, and the fields of its payload (the same names a script reads from `source.event` or $QUESTS_PAYLOAD).
type EventInfo struct {
	Kind   string   `json:"kind"`
	Family string   `json:"family"`
	Title  string   `json:"title"`
	Fields []string `json:"fields"`
}

// Families of events: the entity an event is about.
const (
	FamilyQuest      = "quest"
	FamilyStep       = "step"
	FamilyNote       = "note"
	FamilyValue      = "value"
	FamilyAttachment = "attachment"
)

// Catalog is every kind of event hooks know. It is the one list the interface, the MCP tool and the ports of `source.event` are built from.
var Catalog = []EventInfo{
	{"quest_created", FamilyQuest, "квест создан", []string{"quest_id", "title"}},
	{"quest_appeared", FamilyQuest, "квест появился (по шаблону)", []string{"quest_id", "title"}},
	{"quest_started", FamilyQuest, "квест начался", []string{"quest_id", "title"}},
	{"quest_completed", FamilyQuest, "квест выполнен", []string{"quest_id", "title"}},
	{"status_changed", FamilyQuest, "статус квеста изменён", []string{"quest_id", "title", "detail"}},
	{"quest_failed", FamilyQuest, "квест провален", []string{"quest_id", "title"}},
	{"quest_expired", FamilyQuest, "квест просрочен", []string{"quest_id", "title"}},
	{"quest_deleted", FamilyQuest, "квест удалён", []string{"quest_id", "title"}},
	{"step_completed", FamilyStep, "шаг выполнен", []string{"quest_id", "step_id", "title", "step_title"}},
	{"step_progress", FamilyStep, "прогресс шага", []string{"quest_id", "step_id", "title", "step_title"}},
	{"note_created", FamilyNote, "заметка создана", []string{"note_id", "title"}},
	{"note_updated", FamilyNote, "заметка изменена", []string{"note_id", "title"}},
	{"note_deleted", FamilyNote, "заметка удалена", []string{"note_id", "title"}},
	{"values_changed", FamilyValue, "переменная записана или удалена", []string{"owner_kind", "owner_id", "name", "type", "value"}},
	{"attachment_added", FamilyAttachment, "вложение добавлено", []string{"attachment_id", "owner_type", "owner_id", "title"}},
	{"attachment_revision", FamilyAttachment, "новая версия вложения", []string{"attachment_id", "owner_type", "owner_id", "title"}},
}

// EventFamily is the family of a kind, or "" for a kind the catalog does not know.
func EventFamily(kind string) string {
	for _, e := range Catalog {
		if e.Kind == kind {
			return e.Family
		}
	}
	return ""
}
