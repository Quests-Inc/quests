package repository

import (
	"sync"
	"time"
)

// Cache only immutable per-path commit metadata. Every browse still checks the
// provider with the current credentials before consulting this cache.
type historyKey struct {
	Root                  string
	Workspace, Repository int64
	URL, Ref, Path        string
}
type historyValue struct {
	Commit  Commit
	Expires time.Time
}

var pathHistory = struct {
	sync.Mutex
	items map[historyKey]historyValue
}{items: make(map[historyKey]historyValue)}

func cachedHistory(key historyKey) *Commit {
	pathHistory.Lock()
	defer pathHistory.Unlock()
	value, ok := pathHistory.items[key]
	if !ok {
		return nil
	}
	if time.Now().After(value.Expires) {
		delete(pathHistory.items, key)
		return nil
	}
	copy := value.Commit
	return &copy
}
func rememberHistory(key historyKey, commit *Commit) {
	if commit == nil || key.Ref == "" {
		return
	}
	pathHistory.Lock()
	defer pathHistory.Unlock()
	now := time.Now()
	if len(pathHistory.items) >= 2048 {
		for k, v := range pathHistory.items {
			if now.After(v.Expires) {
				delete(pathHistory.items, k)
			}
		}
		if len(pathHistory.items) >= 2048 {
			for k := range pathHistory.items {
				delete(pathHistory.items, k)
				break
			}
		}
	}
	pathHistory.items[key] = historyValue{Commit: *commit, Expires: now.Add(5 * time.Minute)}
}
