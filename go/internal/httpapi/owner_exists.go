package httpapi

import (
	"context"
	"errors"

	"github.com/quests-inc/quests/go/internal/store"
)

// Owner kinds of attachments (the first two are journal entities, the third is a note).
const (
	ownerQuest     = "quest"
	ownerQuestline = "questline"
	ownerNote      = "note"
)

// ownerExists says whether the quest, questline or note an attachment would hang on exists. It reaches into two
// contexts, so it lives in the root and the knowledge handler receives it as a function.
func (s *Server) ownerExists(ctx context.Context, ownerType string, id int64) (bool, error) {
	if ownerType == ownerQuestline {
		return s.Store.Journal().QuestlineExists(ctx, id)
	}
	if ownerType == ownerNote {
		return s.Store.Knowledge().NoteExists(ctx, id)
	}
	_, err := s.Store.Journal().GetQuest(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
