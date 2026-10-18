package execution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// Custom nodes (quest=458): definitions the owner makes, kept per workspace. A script uses one as `custom.<id>`; the
// definition is read when the script is saved (to know the node's ports) and when it runs (to expand it into plain nodes).

// CustomNode is a stored definition with the bookkeeping the API shows.
type CustomNode struct {
	ID          int64               `json:"id"`
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	Version     int                 `json:"version"`
	Inputs      []graph.CustomPort  `json:"inputs"`
	Outputs     []graph.CustomPort  `json:"outputs"`
	Fields      []graph.CustomField `json:"fields"`
	Graph       graph.Graph         `json:"graph"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	// Subjects: the kinds of entity the sources inside read (`step`, `note`…): what a picker offers the node for.
	Subjects []string `json:"subjects"`
	// NeedsRunner: the node holds a Code node.
	NeedsRunner bool `json:"needs_runner"`
	// Problem is why the definition does not resolve at the moment (a node inside it was deleted), or empty.
	Problem string `json:"problem,omitempty"`
}

// Def is the definition the graph package works with.
func (n CustomNode) Def() graph.Custom {
	return graph.Custom{ID: n.ID, Title: n.Title, Description: n.Description, Version: n.Version, Inputs: n.Inputs, Outputs: n.Outputs, Fields: n.Fields, Graph: n.Graph}
}

// ErrCustomVersion: someone saved the node after it was read.
var ErrCustomVersion = errors.New("ноду изменили в другом месте: обновите страницу и повторите")

// CustomNodeInUse is the refusal to delete a node something still uses.
type CustomNodeInUse struct{ By []string }

func (e *CustomNodeInUse) Error() string {
	return "нода используется: " + strings.Join(e.By, ", ")
}

type customBody struct {
	Inputs  []graph.CustomPort  `json:"inputs"`
	Outputs []graph.CustomPort  `json:"outputs"`
	Fields  []graph.CustomField `json:"fields"`
	Graph   graph.Graph         `json:"graph"`
}

func encodeCustom(c graph.Custom) (string, error) {
	b := customBody{Inputs: c.Inputs, Outputs: c.Outputs, Fields: c.Fields, Graph: c.Graph}
	if b.Inputs == nil {
		b.Inputs = []graph.CustomPort{}
	}
	if b.Outputs == nil {
		b.Outputs = []graph.CustomPort{}
	}
	if b.Fields == nil {
		b.Fields = []graph.CustomField{}
	}
	out, err := json.Marshal(b)
	if len(out) > graph.MaxBytes {
		return "", errBad("нода больше 512 КБ")
	}
	return string(out), err
}

const customCols = `id, title, description, version, definition, created_at, updated_at`

func scanCustom(sc rowScanner) (CustomNode, error) {
	var (
		n                CustomNode
		def              string
		created, updated string
	)
	if err := sc.Scan(&n.ID, &n.Title, &n.Description, &n.Version, &def, &created, &updated); err != nil {
		return CustomNode{}, err
	}
	var b customBody
	if err := json.Unmarshal([]byte(def), &b); err != nil {
		return CustomNode{}, fmt.Errorf("нода %d: %w", n.ID, err)
	}
	n.Inputs, n.Outputs, n.Fields, n.Graph = b.Inputs, b.Outputs, b.Fields, b.Graph
	n.CreatedAt, _ = timeutil.ParseFlexible(created)
	n.UpdatedAt, _ = timeutil.ParseFlexible(updated)
	return n, nil
}

// ListCustomNodes returns the workspace's custom nodes, oldest first (so ids read as a sequence).
func (s *Store) ListCustomNodes(ctx context.Context) ([]CustomNode, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+customCols+` FROM custom_node ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomNode{}
	for rows.Next() {
		n, err := scanCustom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	annotate(out)
	return out, nil
}

// annotate says which definitions resolve right now and which need a runner.
func annotate(nodes []CustomNode) {
	customs := make([]graph.Custom, len(nodes))
	for i, n := range nodes {
		customs[i] = n.Def()
	}
	reg, errs := graph.Builtin().WithCustoms(customs)
	for i := range nodes {
		if err := errs[nodes[i].ID]; err != nil {
			nodes[i].Problem = err.Error()
		}
		nodes[i].Subjects = nodes[i].Graph.Subjects()
		nodes[i].NeedsRunner = reg[nodes[i].Def().TypeName()].Contains[graph.KindCode] > 0
	}
}

// GetCustomNode returns one definition.
func (s *Store) GetCustomNode(ctx context.Context, id int64) (CustomNode, error) {
	n, err := scanCustom(s.DB.QueryRowContext(ctx, `SELECT `+customCols+` FROM custom_node WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return CustomNode{}, ErrNotFound
	}
	if err != nil {
		return CustomNode{}, err
	}
	all, err := s.ListCustomNodes(ctx)
	if err != nil {
		return CustomNode{}, err
	}
	for _, a := range all {
		if a.ID == id {
			return a, nil
		}
	}
	return n, nil
}

// Registry is the built-in nodes plus the workspace's custom nodes that resolve, and the definitions to expand with.
func (s *Store) Registry(ctx context.Context) (graph.Registry, []graph.Custom, error) {
	nodes, err := s.ListCustomNodes(ctx)
	if err != nil {
		return nil, nil, err
	}
	if len(nodes) == 0 {
		return graph.Builtin(), nil, nil
	}
	customs := make([]graph.Custom, len(nodes))
	for i, n := range nodes {
		customs[i] = n.Def()
	}
	reg, _ := graph.Builtin().WithCustoms(customs)
	return reg, customs, nil
}

// registryWithout is the registry without one node: what that node's own definition is checked against, so a node can
// neither contain itself nor close a loop through the nodes it uses.
func (s *Store) registryWithout(ctx context.Context, id int64) (graph.Registry, error) {
	nodes, err := s.ListCustomNodes(ctx)
	if err != nil {
		return nil, err
	}
	customs := make([]graph.Custom, 0, len(nodes))
	for _, n := range nodes {
		if n.ID != id {
			customs = append(customs, n.Def())
		}
	}
	reg, _ := graph.Builtin().WithCustoms(customs)
	return reg, nil
}

// CreateCustomNode checks and stores a new definition (c.ID is ignored).
func (s *Store) CreateCustomNode(ctx context.Context, c graph.Custom) (CustomNode, error) {
	c.Title = strings.TrimSpace(c.Title)
	if err := checkCustomText(c); err != nil {
		return CustomNode{}, err
	}
	reg, err := s.registryWithout(ctx, 0)
	if err != nil {
		return CustomNode{}, err
	}
	c.ID = 1 << 40 // not a real id yet; the definition may not use itself anyway
	if err := c.Check(reg); err != nil {
		return CustomNode{}, errBad(err.Error())
	}
	def, err := encodeCustom(c)
	if err != nil {
		return CustomNode{}, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `INSERT INTO custom_node (title, description, version, definition, created_at, updated_at)
		VALUES (?, ?, 1, ?, ?, ?)`, c.Title, c.Description, def, now, now)
	if err != nil {
		return CustomNode{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetCustomNode(ctx, id)
}

func checkCustomText(c graph.Custom) error {
	switch {
	case c.Title == "":
		return errBad("название ноды обязательно")
	case len([]rune(c.Title)) > MaxScriptTitle:
		return errBad("название длиннее 200 символов")
	case len([]rune(c.Description)) > MaxScriptDescription:
		return errBad("описание длиннее 4000 символов")
	}
	return nil
}

// UpdateCustomNode replaces a definition; expected is the version the editor read. Scripts that use the node keep
// pointing at it, and a change to its ports shows as problems when they are opened.
func (s *Store) UpdateCustomNode(ctx context.Context, id int64, c graph.Custom, expected int) (CustomNode, error) {
	cur, err := s.GetCustomNode(ctx, id)
	if err != nil {
		return CustomNode{}, err
	}
	if cur.Version != expected {
		return CustomNode{}, ErrCustomVersion
	}
	c.ID = id
	c.Title = strings.TrimSpace(c.Title)
	if err := checkCustomText(c); err != nil {
		return CustomNode{}, err
	}
	reg, err := s.registryWithout(ctx, id)
	if err != nil {
		return CustomNode{}, err
	}
	if err := c.Check(reg); err != nil {
		return CustomNode{}, errBad(err.Error())
	}
	def, err := encodeCustom(c)
	if err != nil {
		return CustomNode{}, err
	}
	now := timeutil.ToDBUTC(timeutil.NowUTC())
	res, err := s.DB.ExecContext(ctx, `UPDATE custom_node SET title = ?, description = ?, version = version + 1, definition = ?, updated_at = ?
		WHERE id = ? AND version = ?`, c.Title, c.Description, def, now, id, expected)
	if err != nil {
		return CustomNode{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CustomNode{}, ErrCustomVersion
	}
	return s.GetCustomNode(ctx, id)
}

// DeleteCustomNode removes a definition nothing uses.
func (s *Store) DeleteCustomNode(ctx context.Context, id int64) error {
	if _, err := s.GetCustomNode(ctx, id); err != nil {
		return err
	}
	by, err := s.customNodeUsers(ctx, id)
	if err != nil {
		return err
	}
	if len(by) > 0 {
		return &CustomNodeInUse{By: by}
	}
	_, err = s.DB.ExecContext(ctx, `DELETE FROM custom_node WHERE id = ?`, id)
	return err
}

// customNodeUsers names the scripts (current version) and the other custom nodes that hold the node.
func (s *Store) customNodeUsers(ctx context.Context, id int64) ([]string, error) {
	var by []string
	scripts, err := s.ListScripts(ctx)
	if err != nil {
		return nil, err
	}
	for _, sc := range scripts {
		raw, err := s.ScriptGraph(ctx, sc.ID, sc.Version)
		if err != nil {
			return nil, err
		}
		g, err := graph.Decode(raw)
		if err != nil {
			continue
		}
		for _, u := range g.CustomUses() {
			if u == id {
				by = append(by, fmt.Sprintf("скрипт «%s»", sc.Title))
			}
		}
	}
	nodes, err := s.ListCustomNodes(ctx)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		for _, u := range n.Graph.CustomUses() {
			if u == id {
				by = append(by, fmt.Sprintf("нода «%s»", n.Title))
			}
		}
	}
	return by, nil
}

// InstallStarters makes a custom node of every sample that has none yet (found by title) and returns the new ones. The samples
// are the starting library: a script that is one such node does what the sample did, with its entity number and code as fields.
func (s *Store) InstallStarters(ctx context.Context) ([]CustomNode, error) {
	have, err := s.ListCustomNodes(ctx)
	if err != nil {
		return nil, err
	}
	titles := map[string]bool{}
	for _, n := range have {
		titles[n.Title] = true
	}
	made := []CustomNode{}
	for _, sm := range graph.Samples() {
		if titles[sm.Title] {
			continue
		}
		n, err := s.CreateCustomNode(ctx, graph.Starter(sm))
		if err != nil {
			return nil, fmt.Errorf("образец %q: %w", sm.ID, err)
		}
		made = append(made, n)
	}
	return made, nil
}
