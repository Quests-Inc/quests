package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// remapScriptGraphs rewrites the ids inside the imported scripts' graphs and custom nodes' definitions (quest=458).
func (s *Store) remapScriptGraphs(ctx context.Context, tx *sql.Tx, ids map[string]map[int64]int64, written map[string]map[int64]bool, rep *ImportReport) error {
	custom := func(id int64) (int64, bool) { n, ok := ids["custom_node"][id]; return n, ok }
	entity := func(kind string, id int64) (int64, bool) {
		t, ok := kindTable[kind]
		if !ok {
			return 0, false
		}
		n, ok := ids[t][id]
		return n, ok
	}

	type nodeRow struct {
		id  int64
		def string
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, definition FROM custom_node`)
	if err != nil {
		return err
	}
	var nodes []nodeRow
	for rows.Next() {
		var r nodeRow
		if err := rows.Scan(&r.id, &r.def); err != nil {
			rows.Close()
			return err
		}
		nodes = append(nodes, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, r := range nodes {
		if !written["custom_node"][r.id] {
			continue
		}
		var body struct {
			Inputs  json.RawMessage     `json:"inputs"`
			Outputs json.RawMessage     `json:"outputs"`
			Fields  []graph.CustomField `json:"fields"`
			Graph   graph.Graph         `json:"graph"`
		}
		if json.Unmarshal([]byte(r.def), &body) != nil {
			continue // not a definition this build reads: left as it came
		}
		c, dangling := graph.Custom{Fields: body.Fields, Graph: body.Graph}.RemapIDs(custom, entity)
		rep.DanglingRefs += dangling
		body.Fields, body.Graph = c.Fields, c.Graph
		out, err := json.Marshal(body)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE custom_node SET definition = ? WHERE id = ?`, string(out), r.id); err != nil {
			return err
		}
	}

	type verRow struct {
		script  int64
		version int
		graph   string
	}
	vrows, err := tx.QueryContext(ctx, `SELECT script_id, version, graph FROM script_version`)
	if err != nil {
		return err
	}
	var vers []verRow
	for vrows.Next() {
		var r verRow
		if err := vrows.Scan(&r.script, &r.version, &r.graph); err != nil {
			vrows.Close()
			return err
		}
		vers = append(vers, r)
	}
	vrows.Close()
	if err := vrows.Err(); err != nil {
		return err
	}
	for _, r := range vers {
		if !written[fmt.Sprintf("script_version:%d", r.script)][int64(r.version)] {
			continue
		}
		g, err := graph.Decode([]byte(r.graph))
		if err != nil {
			continue
		}
		ng, dangling := g.RemapIDs(custom, entity)
		rep.DanglingRefs += dangling
		out, err := json.Marshal(ng)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE script_version SET graph = ? WHERE script_id = ? AND version = ?`, string(out), r.script, r.version); err != nil {
			return err
		}
	}
	return nil
}
