package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/valirum/quests/go/internal/platform/apiclient"
)

// The API client lives in platform/apiclient (shared with the assistant); the command line keeps its own names.
type (
	Client   = apiclient.Client
	APIError = apiclient.APIError
)

var (
	NewClient  = apiclient.NewClient
	DecodeList = apiclient.DecodeList
	DecodeMap  = apiclient.DecodeMap
	AsInt64    = apiclient.AsInt64
	AsString   = apiclient.AsString
	AsBool     = apiclient.AsBool
)

func Emit(asJSON bool, data any, text string) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		_ = enc.Encode(data)
		return
	}
	if text != "" {
		fmt.Println(text)
	}
}

func EmitError(asJSON bool, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"ok": false, "error": msg})
	} else {
		fmt.Fprintln(os.Stderr, "error:", msg)
	}
	return 1
}
