package store

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/refs"
)

// Edge kinds. Kept few and drawn differently by the UI.
const (
	EdgeStructure  = "structure"  // belongs to: step→quest, attachment→owner, quest→questline, child note→parent
	EdgeReference  = "reference"  // a quest=N / note=N … token in the source's text
	EdgeDependency = "dependency" // order of steps, a step mirroring a quest, an instance of a template
)

// GraphNode is one entity on the map.
type GraphNode struct {
	Key    string `json:"key"` // "quest:5"
	Type   string `json:"type"`
	ID     int64  `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status,omitempty"` // quest status; "done"/"open" for a step
	// Desc is the start of the description (quest, step, note), for the node card.
	Desc string `json:"desc,omitempty"`
	// Progress is "cur/total" of a step; Sort its position within the quest.
	Progress string `json:"progress,omitempty"`
	Sort     int    `json:"sort"`
	// StepCount is how many steps a quest has (all of them, not just visible).
	StepCount int `json:"step_count"`
	// Icon / IconURL / Color: the entity's own for a note or questline, the
	// questline's for a quest (so a card can wear the questline icon).
	Icon    string `json:"icon,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
	Color   string `json:"color,omitempty"`
	// Mime is the detected content type of an attachment, to pick its icon.
	Mime  string `json:"mime,omitempty"`
	Depth int    `json:"depth"` // hops from the root
	// Links counts reference+dependency edges over the whole graph (not just
	// the visible part): 0 on a note or attachment means nothing points to it.
	Links int `json:"links"`
	// Hub marks nodes that are shown but not expanded (a questline would
	// otherwise pull every quest of the line in).
	Hub bool `json:"hub,omitempty"`
}

type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Graph struct {
	Root  string   `json:"root"`
	Roots []string `json:"roots"` // Root first, then extra roots

	Depth     int         `json:"depth"`
	Truncated bool        `json:"truncated"`
	Nodes     []GraphNode `json:"nodes"`
	Edges     []GraphEdge `json:"edges"`
}

const graphMaxNodes = 200

// Description budgets per node card (runes): enough to read, not the whole note.
const (
	descQuest = 3000
	descNote  = 4000
	descStep  = 600
)

// clip keeps the start of a description for the node card, markdown intact,
// cut on a rune boundary.
func clip(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

func nodeKey(t string, id int64) string { return t + ":" + strconv.FormatInt(id, 10) }

var progressCmd = regexp.MustCompile(`progress\s+(\d+)`)

// BuildGraph maps the neighbourhood of one entity (quest, note, questline) up
// to depth hops. Everything is derived from existing data: ownership columns,
// step order, and the reference tokens in titles and descriptions.
func (s *Store) BuildGraph(ctx context.Context, rootType string, rootID int64, depth int) (Graph, error) {
	return s.BuildGraphRoots(ctx, []GraphRoot{{Type: rootType, ID: rootID}}, depth)
}

// GraphRoot names one entity the map is built around.
type GraphRoot struct {
	Type string
	ID   int64
}

// BuildGraphRoots is BuildGraph around several entities at once. The first
// root must exist; extra roots that are gone are skipped.
func (s *Store) BuildGraphRoots(ctx context.Context, roots []GraphRoot, depth int) (Graph, error) {
	if len(roots) == 0 {
		return Graph{}, ErrNotFound
	}
	rootType, rootID := roots[0].Type, roots[0].ID
	if depth < 1 {
		depth = 1
	}
	if depth > 4 {
		depth = 4
	}
	nodes := map[string]*GraphNode{}
	var edges []GraphEdge
	seenEdge := map[string]bool{}
	addNode := func(t string, id int64, title, status string) *GraphNode {
		k := nodeKey(t, id)
		if _, ok := nodes[k]; !ok {
			nodes[k] = &GraphNode{Key: k, Type: t, ID: id, Title: title, Status: status, Depth: -1}
		}
		return nodes[k]
	}
	addEdge := func(from, to, kind string) {
		if from == to {
			return
		}
		id := from + ">" + to + ">" + kind
		if seenEdge[id] {
			return
		}
		seenEdge[id] = true
		edges = append(edges, GraphEdge{From: from, To: to, Kind: kind})
	}

	scan := func(q string, fn func(row interface{ Scan(...any) error }) error) error {
		rows, err := s.DB.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := fn(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}

	// Entities.
	questLine := map[int64]int64{}
	type iconSet struct{ icon, url, color string }
	lineIcon := map[int64]iconSet{}
	if err := scan(`SELECT id, title, status, questline_id, COALESCE(source,''), description FROM quest`, func(r interface{ Scan(...any) error }) error {
		var id int64
		var title, status, source, desc string
		var line *int64
		if err := r.Scan(&id, &title, &status, &line, &source, &desc); err != nil {
			return err
		}
		addNode("quest", id, title, status).Desc = clip(desc, descQuest)
		if line != nil {
			questLine[id] = *line
			addEdge(nodeKey("questline", *line), nodeKey("quest", id), EdgeStructure)
		}
		var tid int64
		if n, _ := fmt.Sscanf(source, "template:%d", &tid); n == 1 && tid > 0 {
			addEdge(nodeKey("quest", id), nodeKey("template", tid), EdgeDependency)
		}
		return nil
	}); err != nil {
		return Graph{}, err
	}
	if err := scan(`SELECT id, title, icon, color, custom_icon FROM questline`, func(r interface{ Scan(...any) error }) error {
		var id int64
		var title, icon, color string
		var custom *string
		if err := r.Scan(&id, &title, &icon, &color, &custom); err != nil {
			return err
		}
		is := iconSet{icon: icon, color: color}
		if custom != nil && *custom != "" {
			is.url = fmt.Sprintf("/api/questlines/%d/icon", id)
		}
		lineIcon[id] = is
		n := addNode("questline", id, title, "")
		n.Icon, n.IconURL, n.Color = is.icon, is.url, is.color
		return nil
	}); err != nil {
		return Graph{}, err
	}
	if err := scan(`SELECT id, title FROM questtemplate`, func(r interface{ Scan(...any) error }) error {
		var id int64
		var title string
		if err := r.Scan(&id, &title); err != nil {
			return err
		}
		addNode("template", id, title, "")
		return nil
	}); err != nil {
		return Graph{}, err
	}
	if err := scan(`SELECT id, title, parent_id, description, icon, color, custom_icon FROM note`, func(r interface{ Scan(...any) error }) error {
		var id int64
		var title, desc string
		var parent *int64
		var icon, color, custom *string
		if err := r.Scan(&id, &title, &parent, &desc, &icon, &color, &custom); err != nil {
			return err
		}
		nn := addNode("note", id, title, "")
		nn.Desc = clip(desc, descNote)
		if icon != nil {
			nn.Icon = *icon
		}
		if color != nil {
			nn.Color = *color
		}
		if custom != nil && *custom != "" {
			nn.IconURL = fmt.Sprintf("/api/notes/%d/icon", id)
		}
		if parent != nil {
			addEdge(nodeKey("note", *parent), nodeKey("note", id), EdgeStructure)
		}
		return nil
	}); err != nil {
		return Graph{}, err
	}
	type stepRow struct {
		id, quest int64
		wait      bool
		mode, cmd string
	}
	var steps []stepRow
	if err := scan(`SELECT id, quest_id, title, progress_current, progress_total, wait_previous,
			COALESCE(run_mode,''), COALESCE(check_command,''), description FROM queststep
			ORDER BY quest_id, sort_order, id`, func(r interface{ Scan(...any) error }) error {
		var sr stepRow
		var title, desc string
		var cur, total int
		var wait int
		if err := r.Scan(&sr.id, &sr.quest, &title, &cur, &total, &wait, &sr.mode, &sr.cmd, &desc); err != nil {
			return err
		}
		sr.wait = wait != 0
		st := "open"
		if total > 0 && cur >= total {
			st = "done"
		}
		sn := addNode("step", sr.id, title, st)
		sn.Desc = clip(desc, descStep)
		sn.Progress = fmt.Sprintf("%d/%d", cur, total)
		sn.Sort = len(steps)
		addEdge(nodeKey("quest", sr.quest), nodeKey("step", sr.id), EdgeStructure)
		steps = append(steps, sr)
		return nil
	}); err != nil {
		return Graph{}, err
	}
	for _, st := range steps {
		if qn := nodes[nodeKey("quest", st.quest)]; qn != nil {
			qn.StepCount++
		}
	}
	for i, st := range steps {
		if st.wait && i > 0 && steps[i-1].quest == st.quest {
			// Predecessor → dependent, so the arrow follows the order of work.
			addEdge(nodeKey("step", steps[i-1].id), nodeKey("step", st.id), EdgeDependency)
		}
		if m := progressCmd.FindStringSubmatch(st.cmd); m != nil {
			if qid, err := strconv.ParseInt(m[1], 10, 64); err == nil {
				addEdge(nodeKey("step", st.id), nodeKey("quest", qid), EdgeDependency)
			}
		}
	}
	if err := scan(`SELECT id, owner_type, owner_id, filename, content_type_detected FROM attachment WHERE hidden = 0`, func(r interface{ Scan(...any) error }) error {
		var id, owner int64
		var otype, name, mime string
		if err := r.Scan(&id, &otype, &owner, &name, &mime); err != nil {
			return err
		}
		addNode("attachment", id, name, "").Mime = mime
		addEdge(nodeKey(otype, owner), nodeKey("attachment", id), EdgeStructure)
		return nil
	}); err != nil {
		return Graph{}, err
	}

	for qid, lid := range questLine {
		if qn, ok := nodes[nodeKey("quest", qid)]; ok {
			qn.Icon, qn.IconURL, qn.Color = lineIcon[lid].icon, lineIcon[lid].url, lineIcon[lid].color
		}
	}

	// References from text.
	corpus, err := s.ListLinkCorpus(ctx)
	if err != nil {
		return Graph{}, err
	}
	for _, doc := range corpus {
		from := nodeKey(doc.Kind, doc.ID)
		for _, ref := range refs.Parse(doc.Body) {
			to := nodeKey(ref.Kind, ref.ID)
			if _, ok := nodes[to]; ok {
				addEdge(from, to, EdgeReference)
			}
		}
	}

	root := nodeKey(rootType, rootID)
	if _, ok := nodes[root]; !ok {
		return Graph{}, ErrNotFound
	}

	// Edges whose endpoints both exist; adjacency over them.
	adj := map[string][]string{}
	links := map[string]int{}
	kept := edges[:0]
	for _, e := range edges {
		if nodes[e.From] == nil || nodes[e.To] == nil {
			continue
		}
		kept = append(kept, e)
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
		if e.Kind != EdgeStructure {
			links[e.From]++
			links[e.To]++
		}
	}
	edges = kept

	// Breadth-first from the root. A questline is shown but not expanded: it
	// would pull in every quest of the line. The root itself always expands.
	g := Graph{Root: root, Depth: depth, Roots: []string{root}}
	nodes[root].Depth = 0
	order := []string{root}
	for _, x := range roots[1:] {
		k := nodeKey(x.Type, x.ID)
		if n, ok := nodes[k]; ok && n.Depth < 0 {
			n.Depth = 0
			order = append(order, k)
			g.Roots = append(g.Roots, k)
		}
	}
	for i := 0; i < len(order); i++ {
		cur := nodes[order[i]]
		if cur.Depth >= depth {
			continue
		}
		if cur.Type == "questline" && cur.Depth != 0 {
			cur.Hub = true
			continue
		}
		next := append([]string(nil), adj[cur.Key]...)
		sort.Strings(next)
		for _, k := range next {
			if nodes[k].Depth >= 0 {
				continue
			}
			if len(order) >= graphMaxNodes {
				g.Truncated = true
				break
			}
			nodes[k].Depth = cur.Depth + 1
			order = append(order, k)
		}
	}
	in := map[string]bool{}
	g.Edges = []GraphEdge{} // JSON [] rather than null for a lone card
	for _, k := range order {
		n := *nodes[k]
		n.Links = links[k]
		g.Nodes = append(g.Nodes, n)
		in[k] = true
	}
	for _, e := range edges {
		if in[e.From] && in[e.To] {
			g.Edges = append(g.Edges, e)
		}
	}
	return g, nil
}
