package execution

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// Limits of a script's text fields.
const (
	MaxScriptTitle       = 200
	MaxScriptDescription = 4000
)

// ErrScriptVersion: someone saved the script after it was read.
var ErrScriptVersion = errors.New("скрипт изменили в другом месте: обновите страницу и повторите")

// GraphError carries every problem found in a graph, so an editor can mark the nodes.
type GraphError struct{ Problems []graph.Problem }

func (e *GraphError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Error()
	}
	return "граф не прошёл проверку: " + strings.Join(parts, "; ")
}

// Script is a stored script as the API shows it. Graph is set when one script is read, not in a list.
type Script struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Version     int      `json:"version"`
	Subjects    []string `json:"subjects"`
	// Purpose is what the script is for, chosen by hand: one of ScriptPurposes, or "" (unsorted).
	Purpose     string    `json:"purpose"`
	NeedsRunner bool      `json:"needs_runner"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Template is the id of the sample whose skeleton (nodes and links, not their parameters) the current graph still has.
	Template string          `json:"template,omitempty"`
	Graph    json.RawMessage `json:"graph,omitempty"`
}

// ScriptPurposes are the values Purpose may take besides "".
var ScriptPurposes = []string{"step", "note", "template", "hook"}

func checkPurpose(p string) error {
	if p == "" {
		return nil
	}
	for _, v := range ScriptPurposes {
		if p == v {
			return nil
		}
	}
	return errBad("назначение скрипта: step, note, template, hook или пусто")
}

// ScriptUpdate is a change to a script. Graph nil leaves the graph as it is; ExpectedVersion is the version the editor
// read, so two editors cannot overwrite each other.
type ScriptUpdate struct {
	Title           *string
	Description     *string
	Purpose         *string
	Graph           []byte
	ExpectedVersion int
}

// prepared is a graph that passed the checks, in its canonical form and with what the list needs.
type prepared struct {
	json []byte
	// logic is the graph without the editor's positions: a change that leaves it as it was is a move, not a new version.
	logic       []byte
	subjects    string
	needsRunner bool
}

func prepareGraph(raw []byte, reg graph.Registry) (prepared, error) {
	g, err := graph.Decode(raw)
	if err != nil {
		return prepared{}, errBad(err.Error())
	}
	if ps := g.Validate(reg); len(ps) > 0 {
		return prepared{}, &GraphError{Problems: ps}
	}
	canon, err := json.Marshal(g)
	if err != nil {
		return prepared{}, err
	}
	logic, err := json.Marshal(g.WithoutLayout())
	if err != nil {
		return prepared{}, err
	}
	return prepared{json: canon, logic: logic, subjects: strings.Join(g.Subjects(), ","), needsRunner: g.RunsOnRunnerIn(reg)}, nil
}

// sameLogic reports whether a stored graph does what the prepared one does, wherever its nodes sit.
func sameLogic(stored []byte, p prepared) bool {
	g, err := graph.Decode(stored)
	if err != nil {
		return false
	}
	logic, err := json.Marshal(g.WithoutLayout())
	return err == nil && bytes.Equal(logic, p.logic)
}

func checkScriptText(title, description string) (string, error) {
	title = strings.TrimSpace(title)
	switch {
	case title == "":
		return "", errBad("название скрипта обязательно")
	case utf8.RuneCountInString(title) > MaxScriptTitle:
		return "", errBad("название длиннее 200 символов")
	case utf8.RuneCountInString(description) > MaxScriptDescription:
		return "", errBad("описание длиннее 4000 символов")
	}
	return title, nil
}

// CreateScript stores a new script at version 1. The graph is checked first; a bad one is a *GraphError.
func (s *Store) CreateScript(ctx context.Context, title, description string, graphJSON []byte) (Script, error) {
	return s.CreateScriptFor(ctx, title, description, "", graphJSON)
}

// CreateScriptFor is CreateScript with the purpose the script is made for.
func (s *Store) CreateScriptFor(ctx context.Context, title, description, purpose string, graphJSON []byte) (Script, error) {
	title, err := checkScriptText(title, description)
	if err != nil {
		return Script{}, err
	}
	if err := checkPurpose(purpose); err != nil {
		return Script{}, err
	}
	reg, _, err := s.Registry(ctx)
	if err != nil {
		return Script{}, err
	}
	p, err := prepareGraph(graphJSON, reg)
	if err != nil {
		return Script{}, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Script{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO script (title, description, purpose, version, subjects, needs_runner, created_at, updated_at)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?)`, title, description, purpose, p.subjects, boolInt(p.needsRunner), now, now)
	if err != nil {
		return Script{}, err
	}
	id, _ := res.LastInsertId()
	if _, err := tx.ExecContext(ctx, `INSERT INTO script_version (script_id, version, graph, created_at) VALUES (?, 1, ?, ?)`,
		id, string(p.json), now); err != nil {
		return Script{}, err
	}
	if err := tx.Commit(); err != nil {
		return Script{}, err
	}
	return s.GetScript(ctx, id)
}

const scriptCols = `id, title, description, purpose, version, subjects, needs_runner, created_at, updated_at`

func scanScript(sc rowScanner) (Script, error) {
	var (
		sc2              Script
		subjects         string
		needs            int
		created, updated string
	)
	if err := sc.Scan(&sc2.ID, &sc2.Title, &sc2.Description, &sc2.Purpose, &sc2.Version, &subjects, &needs, &created, &updated); err != nil {
		return Script{}, err
	}
	sc2.Subjects = []string{}
	if subjects != "" {
		sc2.Subjects = strings.Split(subjects, ",")
	}
	sc2.NeedsRunner = needs == 1
	sc2.CreatedAt, _ = timeutil.ParseFlexible(created)
	sc2.UpdatedAt, _ = timeutil.ParseFlexible(updated)
	return sc2, nil
}

// ListScripts returns the workspace's scripts, newest change first, without their graphs.
func (s *Store) ListScripts(ctx context.Context) ([]Script, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+scriptCols+` FROM script ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Script{}
	for rows.Next() {
		sc, err := scanScript(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

// MarkTemplates sets Template on each script whose current graph has the skeleton of one of the samples. Samples that
// share a skeleton (two ways to poll a number) are told apart by how many nodes still carry the sample's own parameters.
func (s *Store) MarkTemplates(ctx context.Context, list []Script) error {
	samples := graph.Samples()
	skeletons := make([]string, len(samples))
	for i, sm := range samples {
		skeletons[i] = sm.Graph.Structure()
	}
	for i := range list {
		raw, err := s.ScriptGraph(ctx, list[i].ID, list[i].Version)
		if err != nil {
			return err
		}
		g, err := graph.Decode(raw)
		if err != nil {
			continue // a stored graph that no longer decodes has no template
		}
		skeleton, best := g.Structure(), -1
		for j, sm := range samples {
			if skeletons[j] != skeleton {
				continue
			}
			if score := sameParams(g, sm.Graph); score > best {
				best, list[i].Template = score, sm.ID
			}
		}
	}
	return nil
}

// sameParams counts the nodes of a that have the parameters b gives the node with the same id.
func sameParams(a, b graph.Graph) int {
	want := map[int]string{}
	for _, n := range b.Nodes {
		want[n.ID] = compactJSON(n.Params)
	}
	score := 0
	for _, n := range a.Nodes {
		if w, ok := want[n.ID]; ok && w == compactJSON(n.Params) {
			score++
		}
	}
	return score
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

// GetScript returns one script with its current graph.
func (s *Store) GetScript(ctx context.Context, id int64) (Script, error) {
	sc, err := scanScript(s.DB.QueryRowContext(ctx, `SELECT `+scriptCols+` FROM script WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Script{}, ErrNotFound
	}
	if err != nil {
		return Script{}, err
	}
	g, err := s.ScriptGraph(ctx, id, sc.Version)
	if err != nil {
		return Script{}, err
	}
	sc.Graph = g
	return sc, nil
}

// ScriptGraph returns the graph of one version of a script: a job queued with a version runs that one.
func (s *Store) ScriptGraph(ctx context.Context, id int64, version int) (json.RawMessage, error) {
	var g string
	err := s.DB.QueryRowContext(ctx, `SELECT graph FROM script_version WHERE script_id = ? AND version = ?`, id, version).Scan(&g)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(g), nil
}

// UpdateScript changes the text fields and/or the graph. A changed graph is a new version; a text-only change is not.
func (s *Store) UpdateScript(ctx context.Context, id int64, in ScriptUpdate) (Script, error) {
	cur, err := s.GetScript(ctx, id)
	if err != nil {
		return Script{}, err
	}
	if in.ExpectedVersion != cur.Version {
		return Script{}, ErrScriptVersion
	}
	title, description := cur.Title, cur.Description
	if in.Title != nil {
		title = *in.Title
	}
	if in.Description != nil {
		description = *in.Description
	}
	if title, err = checkScriptText(title, description); err != nil {
		return Script{}, err
	}
	purpose := cur.Purpose
	if in.Purpose != nil {
		purpose = *in.Purpose
		if err := checkPurpose(purpose); err != nil {
			return Script{}, err
		}
	}

	version, subjects, needs := cur.Version, strings.Join(cur.Subjects, ","), cur.NeedsRunner
	var newGraph []byte
	var moveOnly bool
	if in.Graph != nil {
		reg, _, err := s.Registry(ctx)
		if err != nil {
			return Script{}, err
		}
		p, err := prepareGraph(in.Graph, reg)
		if err != nil {
			return Script{}, err
		}
		switch {
		case bytes.Equal(p.json, cur.Graph):
		case sameLogic(cur.Graph, p):
			// Only the positions changed: the current version is rewritten in place, so moving nodes about does not pile up
			// versions (a queued job runs the same logic whichever it reads).
			newGraph, moveOnly = p.json, true
		default:
			version++
			subjects, needs, newGraph = p.subjects, p.needsRunner, p.json
		}
	}

	now := timeutil.ToDBUTC(timeutil.NowUTC())
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Script{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE script SET title = ?, description = ?, purpose = ?, version = ?, subjects = ?, needs_runner = ?, updated_at = ?
		WHERE id = ? AND version = ?`, title, description, purpose, version, subjects, boolInt(needs), now, id, cur.Version)
	if err != nil {
		return Script{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Script{}, ErrScriptVersion // saved by someone else between the read and the write
	}
	if newGraph != nil && moveOnly {
		if _, err := tx.ExecContext(ctx, `UPDATE script_version SET graph = ? WHERE script_id = ? AND version = ?`, string(newGraph), id, version); err != nil {
			return Script{}, err
		}
	} else if newGraph != nil {
		if _, err := tx.ExecContext(ctx, `INSERT INTO script_version (script_id, version, graph, created_at) VALUES (?, ?, ?, ?)`,
			id, version, string(newGraph), now); err != nil {
			return Script{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Script{}, err
	}
	return s.GetScript(ctx, id)
}

// DeleteScript removes a script and all its versions.
func (s *Store) DeleteScript(ctx context.Context, id int64) error {
	if uses, err := s.ScriptUses(ctx, id); err != nil {
		return err
	} else if len(uses) > 0 {
		return &ScriptInUse{Uses: uses}
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM script WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
