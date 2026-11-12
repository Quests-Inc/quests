package mcp

// deadlineFields has one shape for either source; changing the source does not change window/expiry.
type deadlineFields struct {
	DeadlineAt         *string `json:"deadline_at,omitempty" desc:"fixed UTC instant; clear_event when detaching from an event"`
	DurationSeconds    *int    `json:"duration_seconds,omitempty" desc:"urgency window before due, for either source; 0 means no window"`
	ExpireOnDeadline   *bool   `json:"expire_on_deadline,omitempty" desc:"new quest default true; independent of window, false keeps it active after due"`
	EventID            *int64  `json:"event_id,omitempty" desc:"live calendar-event binding; see event tool"`
	EventEdge          *string `json:"event_edge,omitempty" enum:"start,end"`
	EventDir           *string `json:"event_dir,omitempty" enum:"before,after"`
	EventOffsetSeconds *int    `json:"event_offset_seconds,omitempty" desc:"nonnegative offset from the chosen event edge, default 0"`
}

func (f deadlineFields) put(body map[string]any) error {
	if f.EventID != nil && f.DeadlineAt != nil {
		return &Err{422, "choose event_id or a fixed deadline_at"}
	}
	if f.DeadlineAt != nil {
		body["deadline_at"] = *f.DeadlineAt
	}
	if f.DurationSeconds != nil {
		body["duration_seconds"] = *f.DurationSeconds
	}
	if f.ExpireOnDeadline != nil {
		body["expire_on_deadline"] = *f.ExpireOnDeadline
	}
	if f.EventID != nil {
		body["event_id"] = *f.EventID
	}
	if f.EventEdge != nil {
		body["event_edge"] = *f.EventEdge
	}
	if f.EventDir != nil {
		body["event_dir"] = *f.EventDir
	}
	if f.EventOffsetSeconds != nil {
		body["event_offset_seconds"] = *f.EventOffsetSeconds
	}
	return nil
}
