package schedule

import (
	"context"
	"errors"
	"github.com/quests-inc/quests/go/internal/execution/graph"
)

func (r *CheckRunner) embeddedNoteReader(kind string, current int64) graph.NoteReader {
	return func(ctx context.Context, id int64) (string, string, error) {
		if id == 0 && kind == "note" {
			id = current
		}
		if id <= 0 {
			return "", "", errors.New("источник заметки: укажите id")
		}
		n, err := r.Store.Knowledge.GetNote(ctx, id)
		if err != nil {
			return "", "", err
		}
		if n.IsPrivate {
			return "", "", errors.New("приватные заметки недоступны источнику вставки")
		}
		return n.Title, n.Description, nil
	}
}
