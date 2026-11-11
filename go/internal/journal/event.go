package journal

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

const (
	defaultEventColor = "#5a8a9a"
	defaultEventIcon  = "calendar"
)

var (
	eventColorRe = regexp.MustCompile(`^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	eventIcons   = map[string]bool{
		"calendar": true, "flag": true, "target": true, "scroll": true,
		"map": true, "layers": true, "document": true,
	}
)

const (
	EventEdgeStart = "start"
	EventEdgeEnd   = "end"
	EventDirBefore = "before"
	EventDirAfter  = "after"
)

// Event is a marked interval on the calendar. The interface calls it «Событие».
type Event struct {
	ID          int64
	Title       string
	Description string
	StartsAt    time.Time
	EndsAt      time.Time
	AllDay      bool
	Color       string
	Icon        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// EventRead is an event as the API sends it.
type EventRead struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartsAt    string `json:"starts_at"`
	EndsAt      string `json:"ends_at"`
	AllDay      bool   `json:"all_day"`
	Color       string `json:"color"`
	Icon        string `json:"icon"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func EventToRead(o Event) EventRead {
	read := EventRead{
		ID:          o.ID,
		Title:       o.Title,
		Description: o.Description,
		StartsAt:    isoString(o.StartsAt),
		EndsAt:      isoString(o.EndsAt),
		AllDay:      o.AllDay,
		Color:       o.Color,
		Icon:        o.Icon,
		CreatedAt:   isoString(o.CreatedAt),
		UpdatedAt:   isoString(o.UpdatedAt),
	}
	return read
}

func isoString(t time.Time) string {
	s := toISO(&t)
	if s == nil {
		return ""
	}
	return *s
}

// ResolveEventDeadline is the quest deadline at an offset from an event edge.
// edge is start or end, dir is before («за») or after («через»), offset is seconds and not negative.
func ResolveEventDeadline(start, end time.Time, edge, dir string, offset int) (time.Time, error) {
	if edge == "" {
		edge = EventEdgeStart
	}
	if dir == "" {
		dir = EventDirBefore
	}
	if edge != EventEdgeStart && edge != EventEdgeEnd {
		return time.Time{}, errors.New("край события: начало или конец")
	}
	if dir != EventDirBefore && dir != EventDirAfter {
		return time.Time{}, errors.New("сдвиг: за или через")
	}
	if offset < 0 {
		return time.Time{}, errors.New("сдвиг не может быть отрицательным")
	}
	base := start
	if edge == EventEdgeEnd {
		base = end
	}
	d := time.Duration(offset) * time.Second
	if dir == EventDirAfter {
		return base.Add(d), nil
	}
	return base.Add(-d), nil
}

func (s *Store) ListEvents(ctx context.Context) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, title, description, starts_at, ends_at, all_day, color, icon, created_at, updated_at
		FROM event
		ORDER BY starts_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		o, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) GetEvent(ctx context.Context, id int64) (Event, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT id, title, description, starts_at, ends_at, all_day, color, icon, created_at, updated_at
		FROM event WHERE id = ?`, id)
	o, err := scanEvent(row)
	if err == sql.ErrNoRows {
		return Event{}, ErrNotFound
	}
	return o, err
}

func (s *Store) CreateEvent(ctx context.Context, o Event) (Event, error) {
	if err := validEvent(&o); err != nil {
		return Event{}, err
	}
	now := timeutil.NowUTC()
	o.CreatedAt, o.UpdatedAt = now, now
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO event (title, description, starts_at, ends_at, all_day, color, icon, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.Title, o.Description, timeutil.ToDBUTC(o.StartsAt), timeutil.ToDBUTC(o.EndsAt), boolInt(o.AllDay),
		o.Color, o.Icon,
		timeutil.ToDBUTC(o.CreatedAt), timeutil.ToDBUTC(o.UpdatedAt),
	)
	if err != nil {
		return Event{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, id)
}

func (s *Store) UpdateEvent(ctx context.Context, o Event) (Event, error) {
	if err := validEvent(&o); err != nil {
		return Event{}, err
	}
	o.UpdatedAt = timeutil.NowUTC()
	res, err := s.DB.ExecContext(ctx, `
		UPDATE event SET title=?, description=?, starts_at=?, ends_at=?, all_day=?, color=?, icon=?, updated_at=?
		WHERE id=?`,
		o.Title, o.Description, timeutil.ToDBUTC(o.StartsAt), timeutil.ToDBUTC(o.EndsAt), boolInt(o.AllDay),
		o.Color, o.Icon,
		timeutil.ToDBUTC(o.UpdatedAt), o.ID,
	)
	if err != nil {
		return Event{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Event{}, ErrNotFound
	}
	if err := s.recomputeEventQuests(ctx, o); err != nil {
		return Event{}, err
	}
	return s.GetEvent(ctx, o.ID)
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Keep each quest's resolved deadline. Drop only the phrase.
	if _, err := tx.ExecContext(ctx, `
		UPDATE quest SET event_id=NULL, event_edge=NULL, event_dir=NULL, event_offset_seconds=NULL
		WHERE event_id=?`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM event WHERE id=?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// ApplyEvent sets or clears a quest's event phrase and writes the resolved deadline.
// A set phrase has no urgency window: the window belongs to the absolute-deadline form.
func (s *Store) ApplyEvent(ctx context.Context, q *Quest, id *int64, edge, dir string, offset int) error {
	if id == nil {
		q.EventID = nil
		q.EventEdge = ""
		q.EventDir = ""
		q.EventOffset = nil
		q.EventTitle = nil
		return nil
	}
	o, err := s.GetEvent(ctx, *id)
	if err != nil {
		return err
	}
	if edge == "" {
		edge = EventEdgeStart
	}
	if dir == "" {
		dir = EventDirBefore
	}
	at, err := ResolveEventDeadline(o.StartsAt, o.EndsAt, edge, dir, offset)
	if err != nil {
		return err
	}
	at = timeutil.EnsureUTC(at)
	zero := 0
	q.DeadlineAt = &at
	q.DurationSeconds = &zero
	q.EventID = id
	q.EventEdge = edge
	q.EventDir = dir
	q.EventOffset = &offset
	title := o.Title
	q.EventTitle = &title
	return nil
}

func (s *Store) recomputeEventQuests(ctx context.Context, o Event) error {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, event_edge, event_dir, COALESCE(event_offset_seconds, 0)
		FROM quest WHERE event_id = ?`, o.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	type row struct {
		id     int64
		edge   sql.NullString
		dir    sql.NullString
		offset int
	}
	var pending []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.edge, &r.dir, &r.offset); err != nil {
			return err
		}
		pending = append(pending, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_ = rows.Close()
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	for _, r := range pending {
		edge, dir := "", ""
		if r.edge.Valid {
			edge = r.edge.String
		}
		if r.dir.Valid {
			dir = r.dir.String
		}
		at, err := ResolveEventDeadline(o.StartsAt, o.EndsAt, edge, dir, r.offset)
		if err != nil {
			return err
		}
		if _, err := s.DB.ExecContext(ctx, `
			UPDATE quest SET deadline_at=?, duration_seconds=0, updated_at=? WHERE id=?`,
			timeutil.ToDBUTC(at), now, r.id); err != nil {
			return err
		}
	}
	return nil
}

func validEvent(o *Event) error {
	o.Title = strings.TrimSpace(o.Title)
	if o.Title == "" {
		return errors.New("нужно название")
	}
	o.StartsAt = timeutil.EnsureUTC(o.StartsAt)
	o.EndsAt = timeutil.EnsureUTC(o.EndsAt)
	if o.EndsAt.Before(o.StartsAt) {
		return errors.New("конец раньше начала")
	}
	if !eventColorRe.MatchString(o.Color) {
		o.Color = defaultEventColor
	}
	if !eventIcons[o.Icon] {
		o.Icon = defaultEventIcon
	}
	return nil
}

func scanEvent(row rowScanner) (Event, error) {
	var o Event
	var starts, ends, created, updated string
	var allDay int
	err := row.Scan(&o.ID, &o.Title, &o.Description, &starts, &ends, &allDay, &o.Color, &o.Icon, &created, &updated)
	if err != nil {
		return o, err
	}
	o.AllDay = allDay != 0
	var perr error
	if o.StartsAt, perr = timeutil.ParseFlexible(starts); perr != nil {
		return o, perr
	}
	if o.EndsAt, perr = timeutil.ParseFlexible(ends); perr != nil {
		return o, perr
	}
	if o.CreatedAt, perr = timeutil.ParseFlexible(created); perr != nil {
		return o, perr
	}
	if o.UpdatedAt, perr = timeutil.ParseFlexible(updated); perr != nil {
		return o, perr
	}
	return o, nil
}

// QuestToEvent copies the quest's title, description and deadline onto a new event, then moves the quest to the trash
// so its steps leave the journal. A deadline becomes one moment; without one the event is all of today (UTC).
func (s *Store) QuestToEvent(ctx context.Context, id int64) (Event, error) {
	q, err := s.GetQuest(ctx, id)
	if err != nil {
		return Event{}, err
	}
	now := timeutil.NowUTC()
	ev := Event{
		Title:       q.Title,
		Description: q.Description,
		Color:       defaultEventColor,
		Icon:        defaultEventIcon,
	}
	if q.DeadlineAt != nil {
		at := timeutil.EnsureUTC(*q.DeadlineAt)
		ev.StartsAt, ev.EndsAt = at, at
	} else {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		ev.StartsAt, ev.EndsAt, ev.AllDay = day, day.Add(24*time.Hour), true
	}
	saved, err := s.CreateEvent(ctx, ev)
	if err != nil {
		return Event{}, err
	}
	if err := s.TrashQuest(ctx, id, q.Title); err != nil {
		_ = s.DeleteEvent(ctx, saved.ID)
		return Event{}, err
	}
	return saved, nil
}

func emptyStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
