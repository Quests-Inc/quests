package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv sets process env vars from ROOT/.env for values not already
// set — mirrors python-dotenv's default (non-override) behavior. Docker
// already injects these via compose's env_file:, so this only matters for
// bare `quests-server` run in local dev.
func LoadDotenv(root string) {
	path := filepath.Join(root, ".env")
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		// Strip a trailing " # comment" on unquoted values (avoids the exact
		// python-dotenv footgun that once turned GROQ_API_KEY into prose).
		if !strings.HasPrefix(val, `"`) && !strings.HasPrefix(val, `'`) {
			if i := strings.Index(val, " #"); i >= 0 {
				val = strings.TrimSpace(val[:i])
			}
		}
		val = strings.Trim(val, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, val)
	}
}
