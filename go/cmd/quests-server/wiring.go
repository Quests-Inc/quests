package main

import (
	"context"

	"github.com/valirum/quests/go/internal/platform/metrics"
	"github.com/valirum/quests/go/internal/store"
)

// statsSource adapts the data store to what the metrics collector asks for (metrics knows no store).
type statsSource struct{ st *store.Store }

func (s statsSource) AggregateStats(ctx context.Context) (metrics.Stats, error) {
	st, err := s.st.AggregateStats(ctx)
	// the field sets are identical, so a drift between the two shows up as a compile error here
	return metrics.Stats(st), err
}
