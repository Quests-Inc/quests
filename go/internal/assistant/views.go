package assistant

import "github.com/quests-inc/quests/go/internal/llmassist"

// LLMView is the account's assistant settings as the settings page shows them.
type LLMView struct {
	Providers      []llmassist.Provider `json:"providers"`
	Provider       string               `json:"provider"`
	BaseURL        string               `json:"base_url"`
	Model          string               `json:"model"`
	HasKey         bool                 `json:"has_key"`
	KeyHint        string               `json:"key_hint"`
	Source         string               `json:"source"` // own | server | none
	ServerFallback bool                 `json:"server_fallback"`
	Storage        bool                 `json:"storage"` // can this instance keep a personal key at all
	TextLimit      int                  `json:"text_limit"`
}
