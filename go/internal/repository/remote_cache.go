package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/egress"
)

// Private provider responses live only in bounded server memory. The key includes
// the workspace, repository URL and credential fingerprint; permission
// and credential reads happen before every lookup. Never cache error bodies.
type remoteKey struct {
	Root                      string
	Workspace, Repository     int64
	URL, Credential, Endpoint string
}
type remoteValue struct {
	Body           []byte
	ETag           string
	Fresh, Expires time.Time
}

var providerCache = struct {
	sync.Mutex
	items    map[remoteKey]remoteValue
	pending  map[remoteKey]chan struct{}
	cooldown map[string]time.Time
	bytes    int
}{items: make(map[remoteKey]remoteValue), pending: make(map[remoteKey]chan struct{}), cooldown: make(map[string]time.Time)}

func credentialID(credential string) string {
	sum := sha256.Sum256([]byte(credential))
	return hex.EncodeToString(sum[:])
}
func (k remoteKey) budget() string {
	host, _ := url.Parse(k.Endpoint)
	return fmt.Sprintf("%s:%d:%s:%s", k.Root, k.Workspace, host.Host, k.Credential)
}
func rateWait(result *egress.Result, now time.Time) time.Time {
	until := now.Add(time.Minute)
	if seconds, err := strconv.Atoi(result.RetryAfter); err == nil && seconds > 0 {
		until = now.Add(time.Duration(seconds) * time.Second)
	} else if date, err := http.ParseTime(result.RetryAfter); err == nil && date.After(now) {
		until = date
	}
	if result.RateRemaining == "0" {
		if reset, err := strconv.ParseInt(result.RateReset, 10, 64); err == nil && time.Unix(reset, 0).After(until) {
			until = time.Unix(reset, 0)
		}
	}
	return until
}

func cachedRemote(ctx context.Context, key remoteKey, refresh bool, fetch func(string) (*egress.Result, error), out any) error {
	for {
		now := time.Now()
		providerCache.Lock()
		old, ok := providerCache.items[key]
		if ok && now.After(old.Expires) {
			providerCache.bytes -= len(old.Body)
			delete(providerCache.items, key)
			ok = false
		}
		if ok && !refresh && now.Before(old.Fresh) {
			providerCache.Unlock()
			return json.Unmarshal(old.Body, out)
		}
		if until := providerCache.cooldown[key.budget()]; now.Before(until) {
			providerCache.Unlock()
			return fmt.Errorf("лимит API: повторите после %s", until.UTC().Format(time.RFC3339))
		}
		if pending := providerCache.pending[key]; pending != nil {
			providerCache.Unlock()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pending:
				refresh = false
				continue
			}
		}
		pending := make(chan struct{})
		providerCache.pending[key] = pending
		providerCache.Unlock()
		etag := ""
		if ok {
			etag = old.ETag
		}
		result, err := fetch(etag)
		if err == nil {
			if result.Status == http.StatusNotModified && ok {
				result.Body = old.Body
				result.ETag = old.ETag
			} else if result.Status != http.StatusOK {
				err = fmt.Errorf("хост репозитория вернул HTTP %d", result.Status)
			}
			if err == nil {
				err = json.Unmarshal(result.Body, out)
				if err != nil {
					err = fmt.Errorf("не удалось прочитать ответ хоста")
				}
			}
		}
		providerCache.Lock()
		if result != nil && (result.Status == 429 || result.Status == 403 && (result.RateRemaining == "0" || result.RetryAfter != "" || containsRateLimit(result.Body))) {
			until := rateWait(result, now)
			for budget, expiry := range providerCache.cooldown {
				if now.After(expiry) {
					delete(providerCache.cooldown, budget)
				}
			}
			if len(providerCache.cooldown) < 1024 {
				providerCache.cooldown[key.budget()] = until
			}
			err = fmt.Errorf("лимит API: повторите после %s", until.UTC().Format(time.RFC3339))
		}
		if err == nil && len(result.Body) <= 16<<20 {
			if previous, exists := providerCache.items[key]; exists {
				providerCache.bytes -= len(previous.Body)
				delete(providerCache.items, key)
			}
			for len(providerCache.items) >= 512 || providerCache.bytes+len(result.Body) > 16<<20 {
				for candidate, value := range providerCache.items {
					providerCache.bytes -= len(value.Body)
					delete(providerCache.items, candidate)
					break
				}
			}
			providerCache.items[key] = remoteValue{Body: result.Body, ETag: result.ETag, Fresh: now.Add(10 * time.Minute), Expires: now.Add(30 * time.Minute)}
			providerCache.bytes += len(result.Body)
		}
		delete(providerCache.pending, key)
		close(pending)
		providerCache.Unlock()
		if err != nil {
			return err
		}
		return nil
	}
}
