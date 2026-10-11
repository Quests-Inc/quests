package httpapi

import (
	"net/http"

	"github.com/quests-inc/quests/go/internal/graphdraft"
)

// graphdraftHandler is the map draft's HTTP face, wired to this server (built on first use).
func (s *Server) graphdraftHandler() *graphdraft.Handler {
	s.graphdraftOnce.Do(func() {
		kh := s.knowledgeHTTP()
		s.gdh = &graphdraft.Handler{
			Store:          s.Store,
			WebDAV:         s.WebDAV,
			ClamAV:         s.ClamAV,
			MaxUploadBytes: s.MaxUploadBytes,
			API:            func() http.Handler { return s.routes() },
			WebDAVOK:       kh.WebDAVOK,
			ClamAVOK:       kh.ClamAVOK,
			CheckQuota:     kh.CheckQuota,
		}
	})
	return s.gdh
}
