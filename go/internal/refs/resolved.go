package refs

// Resolved is a kind=ID mention in a text with the title it points at (null when the target is gone).
type Resolved struct {
	Kind  string  `json:"kind"`
	ID    int64   `json:"id"`
	Title *string `json:"title"`
	// QuestID is set for a step: the quest it belongs to.
	QuestID *int64 `json:"quest_id,omitempty"`
	// OwnerType and OwnerID are set for an attachment: what it hangs on.
	OwnerType string `json:"owner_type,omitempty"`
	OwnerID   *int64 `json:"owner_id,omitempty"`
}
