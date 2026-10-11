package httpapi

import (
	"github.com/quests-inc/quests/go/internal/wsarchive"
)

// wsarchiveHandler is the workspace archive's HTTP face, wired to this server.
func (s *Server) wsarchiveHandler() *wsarchive.Handler {
	return &wsarchive.Handler{
		Store:          s.Store,
		DataDir:        s.DataDir,
		WebDAV:         s.WebDAV,
		ClamAV:         s.ClamAV,
		MaxUploadBytes: s.MaxUploadBytes,
		WorkspaceQuota: s.WorkspaceQuota,
		MemberOf:       s.identityHTTP().MemberOf,
		OverQuota:      s.knowledgeHTTP().OverQuota,
	}
}
