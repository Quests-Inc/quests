package graphdraft

import (
	"encoding/json"

	"github.com/quests-inc/quests/go/internal/store"
)

// GraphDraftView is the pending edits of the relations map and the cards it is built around.
type GraphDraftView struct {
	Ops   []store.DraftOp `json:"ops"`
	Roots json.RawMessage `json:"roots" tstype:"{ type: string; id: number }[]"`
}
