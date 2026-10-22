package execution

import (
	"context"
	"errors"
	"strings"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/kernel"
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

// RunnerValueName is the workspace value that names the default runner of embedded scripts: one label, or labels separated by
// commas, tried in turn (the first one online gets the job). Only the owner writes a workspace value.
const RunnerValueName = "runner"

// ResolveEmbedRunner picks where the code of a clicked embedding runs: the link's own choice, else the workspace default, else
// the only runner there is. Every label must be a registered runner of this workspace, so a text cannot name a device that
// is not one of ours.
func (s *Store) ResolveEmbedRunner(ctx context.Context, requested string) (string, error) {
	all, err := s.ListRunners(ctx)
	if err != nil {
		return "", err
	}
	var avail []string
	known := map[string]bool{}
	for _, r := range all {
		if r.Registered && !r.Revoked {
			avail = append(avail, r.Label)
			known[r.Label] = true
		}
	}
	list := strings.Join(avail, ", ")
	raw, from := strings.TrimSpace(requested), "ссылки"
	if raw == "" {
		raw, from = s.workspaceRunner(ctx), "по умолчанию"
	}
	if raw == "" {
		switch len(avail) {
		case 0:
			return "", errors.New("в пространстве нет раннера")
		case 1:
			return avail[0], nil
		}
		return "", errors.New("раннеров несколько (" + list + "): задайте раннер по умолчанию в настройках пространства или ?on=метка")
	}
	norm := kernel.NormalizeRunOn(&raw)
	if norm == nil || !kernel.ValidRunOn(norm) {
		return "", errors.New("раннеры (" + from + "): " + kernel.ErrBadRunOn.Error())
	}
	for _, label := range kernel.SplitRunOn(*norm) {
		if !known[label] {
			return "", errors.New("раннер «" + label + "» (" + from + ") не зарегистрирован (есть: " + list + ")")
		}
	}
	return *norm, nil
}

// workspaceRunner reads the default runner chain, or "".
func (s *Store) workspaceRunner(ctx context.Context) string {
	rows, err := s.listStored(ctx, workspaceOwner(ctx))
	if err != nil {
		return ""
	}
	for _, row := range rows {
		if row.Name == RunnerValueName && row.Type == "str" && row.Formula == "" {
			return row.Value
		}
	}
	return ""
}
