package schedule

import (
	"context"
	"log"
	"time"

	"github.com/valirum/quests/go/internal/platform/webdav"
	"github.com/valirum/quests/go/internal/platform/wsctx"
	"github.com/valirum/quests/go/internal/store"
)

// RunWorkspacePurgeLoop removes workspaces whose deletion grace period is over:
// their attachment files on WebDAV first, then the database rows (every table's
// workspace_id cascades). Hourly.
func RunWorkspacePurgeLoop(ctx context.Context, st *store.Store, wd *webdav.Client) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		PurgeDueWorkspaces(ctx, st, wd)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PurgeDueWorkspaces purges every workspace past its grace period.
func PurgeDueWorkspaces(ctx context.Context, st *store.Store, wd *webdav.Client) {
	ids, err := st.DueForPurge(ctx)
	if err != nil {
		log.Printf("purge: list: %v", err)
		return
	}
	for _, id := range ids {
		wctx := wsctx.With(ctx, id)
		if wd != nil && wd.Configured() {
			paths, err := listWebDAVPaths(wctx, st.DB)
			if err != nil {
				log.Printf("purge: workspace %d attachments: %v", id, err)
				continue
			}
			failed := false
			for _, p := range paths {
				if err := wd.Delete(ctx, p); err != nil && err != webdav.ErrNotFound {
					log.Printf("purge: workspace %d delete %s: %v", id, p, err)
					failed = true
				}
			}
			if failed {
				continue // retry next hour rather than orphan files
			}
		}
		if err := st.PurgeWorkspace(ctx, id); err != nil {
			log.Printf("purge: workspace %d: %v", id, err)
			continue
		}
		log.Printf("purge: workspace %d removed", id)
	}
}
