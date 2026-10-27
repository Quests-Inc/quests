package httpapi

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/quests-inc/quests/go/internal/execution"
	journalhttp "github.com/quests-inc/quests/go/internal/journal/http"
	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// trashRetention is how long a quest or a note stays in the trash before the scheduler erases it for good.
const trashRetention = journalhttp.TrashRetention

// PurgeExpiredTrash erases, in every workspace, the quests, notes, scripts and hooks that have been in the trash for longer than the
// retention: the rows, their attachments (database and WebDAV bytes) and a note's icon file.
func (s *Server) PurgeExpiredTrash(ctx context.Context) {
	cutoff := timeutil.NowUTC().Add(-trashRetention)
	err := s.Store.Identity.ForEachWorkspace(ctx, func(wctx context.Context, ws int64) {
		quests, err := s.Store.Journal.TrashedQuestIDsBefore(wctx, cutoff)
		if err != nil {
			log.Printf("trash: workspace %d quests: %v", ws, err)
		}
		for _, id := range quests {
			if err := s.Store.Journal.PurgeQuest(wctx, id); err != nil {
				log.Printf("trash: purge quest %d: %v", id, err)
				continue
			}
			s.knowledgeHTTP().PurgeOwnerAttachments(wctx, "quest", id)
		}
		notes, err := s.Store.Knowledge.TrashedNoteIDsBefore(wctx, cutoff)
		if err != nil {
			log.Printf("trash: workspace %d notes: %v", ws, err)
		}
		for _, id := range notes {
			icon, _ := s.Store.Knowledge.TrashedNoteIcon(wctx, id)
			if err := s.Store.Knowledge.PurgeNote(wctx, id); err != nil {
				log.Printf("trash: purge note %d: %v", id, err)
				continue
			}
			s.knowledgeHTTP().PurgeOwnerAttachments(wctx, "note", id)
			if icon != "" {
				_ = os.Remove(filepath.Join(s.DataDir, "note-icons", icon))
			}
		}
		for _, kind := range []string{"template", "questline"} {
			items, err := s.Store.ListTrashedContainers(wctx, kind)
			if err != nil {
				log.Printf("trash: workspace %d %s: %v", ws, kind, err)
				continue
			}
			for _, item := range items {
				at, err := timeutil.ParseFlexible(item.DeletedAt)
				if err == nil && at.Before(cutoff) {
					if err := s.purgeContainer(wctx, kind, item.ID); err != nil {
						log.Printf("trash: purge %s %d: %v", kind, item.ID, err)
					}
				}
			}
		}
		// Hooks before scripts: a script's purge takes its trashed hooks along anyway.
		for _, kind := range []string{"hook", "script"} {
			ids, err := s.Store.Execution.TrashedIDsBefore(wctx, kind, cutoff)
			if err != nil {
				log.Printf("trash: workspace %d %ss: %v", ws, kind, err)
			}
			for _, id := range ids {
				if err := s.Store.Execution.PurgeTrashed(wctx, kind, id); err != nil && !errors.Is(err, execution.ErrNotFound) {
					log.Printf("trash: purge %s %d: %v", kind, id, err)
				}
			}
		}
	})
	if err != nil {
		log.Printf("trash: workspaces: %v", err)
	}
}

// RunTrashPurgeLoop runs PurgeExpiredTrash hourly until ctx is done.
func (s *Server) RunTrashPurgeLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		s.PurgeExpiredTrash(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
