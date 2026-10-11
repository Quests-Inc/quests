package httpapi

import (
	"context"

	journalhttp "github.com/valirum/quests/go/internal/journal/http"
)

// journalHTTP is the journal context's HTTP face, wired to this server (built on first use). The things that live in
// other contexts come in as functions: what a mention points at, the attachments of a deleted quest, a template.
func (s *Server) journalHTTP() *journalhttp.Handler {
	s.journalOnce.Do(func() {
		s.jh = &journalhttp.Handler{
			Store:       s.Store.Journal(),
			Hub:         s.Hub,
			DataDir:     s.DataDir,
			ResolveRefs: s.resolveRefs,
			PurgeAttachments: func(ctx context.Context, ownerType string, ownerID int64) {
				s.knowledgeHTTP().PurgeOwnerAttachments(ctx, ownerType, ownerID)
			},
			Template: func(ctx context.Context, id int64) (journalhttp.TemplateRef, error) {
				t, err := s.Store.GetTemplate(ctx, id)
				if err != nil {
					return journalhttp.TemplateRef{}, err
				}
				return journalhttp.TemplateRef{Title: t.Title, Tags: t.Tags}, nil
			},
		}
	})
	return s.jh
}
