package httpapi

import (
	"github.com/quests-inc/quests/go/internal/assistant"
)

// assistantHandler is the assistant's HTTP face, wired to this server.
func (s *Server) assistantHandler() *assistant.Handler {
	return &assistant.Handler{
		Store:         s.Store,
		Secrets:       s.Secrets,
		TryLLM:        s.limiter().TryLLM,
		SelfBase:      s.SelfBase,
		InternalToken: s.InternalToken,
	}
}
