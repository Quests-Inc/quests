package execution

import (
	"context"
	"errors"

	"github.com/quests-inc/quests/go/internal/execution/graph"
)

// EmbedKeyPrefix marks the runs that a text started (`script=N{…}?on=label`): only those are readable by the page that waits.
const EmbedKeyPrefix = "embed:"

// MaxOpenEmbedded caps the runs of embedded scripts waiting for a runner at the same time, per workspace.
const MaxOpenEmbedded = 6

// ErrTooManyEmbedded is the answer when a workspace already has that many waiting.
var ErrTooManyEmbedded = errors.New("уже ждут раннера несколько вставок, подождите")

// EmbedStart describes a click on an embedded script that has a Code node.
type EmbedStart struct {
	ScriptID int64
	Version  int
	Graph    graph.Graph
	Args     map[string]string
	// HostKind and HostID name the entity whose text was clicked; its values are the scope of the formulas and its secrets
	// and values go to the job like they would for its own check. Empty for a text with no owner.
	HostKind string
	HostID   int64
	Label    string
	Scope    *graph.ExprScope
}

// CountOpenEmbedded is the number of embedded runs still waiting for a runner.
func (s *Store) CountOpenEmbedded(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM script_run WHERE state = ? AND run_key LIKE ?`, RunWaiting, EmbedKeyPrefix+"%").Scan(&n)
	return n, err
}
