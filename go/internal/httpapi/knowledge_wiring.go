package httpapi

import (
	"context"

	"github.com/quests-inc/quests/go/internal/journal"
	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
)

// knowledgeHTTP is the knowledge context's HTTP face, wired to this server (built on first use). What the context
// needs from the others is supplied here as functions: mentions are resolved against every kind of entity, the
// change log belongs to the journal, deleting a note removes its attachments.
func (s *Server) knowledgeHTTP() *knowledgehttp.Handler {
	s.knowledgeOnce.Do(func() {
		s.kh = &knowledgehttp.Handler{
			Store:       s.Store.Knowledge(),
			Hub:         s.Hub,
			DataDir:     s.DataDir,
			ResolveRefs: s.resolveRefs,
			PurgeAttachments: func(ctx context.Context, ownerType string, ownerID int64) {
				s.knowledgeHTTP().PurgeOwnerAttachments(ctx, ownerType, ownerID)
			},
			OwnerExists:    s.ownerExists,
			WebDAV:         s.WebDAV,
			ClamAV:         s.ClamAV,
			MaxUploadBytes: s.MaxUploadBytes,
			Quota:          func() int64 { return s.WorkspaceQuota },
			LogEvent: func(ctx context.Context, e knowledgehttp.ChangeEvent) {
				_ = s.Store.Journal().LogEvent(ctx, journal.ChangeEvent{
					Kind: e.Kind, EntityType: e.EntityType, EntityID: e.EntityID,
					Title: e.Title, Detail: e.Detail, Coalesce: e.Coalesce,
					ParentType: e.ParentType, ParentID: e.ParentID,
				})
			},
		}
	})
	return s.kh
}
