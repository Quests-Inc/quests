// Package api is the overlay's HTTP face of the Quests API.
package api

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/overlay/config"
	"github.com/quests-inc/quests/go/internal/platform/apiclient"
)

// Client wraps platform/apiclient for HUD use.
type Client struct {
	*apiclient.Client
}

// New builds a client: QUESTS_API → overlay.json api_base → localhost.
func New(cfg config.Config) *Client {
	base := strings.TrimSpace(os.Getenv("QUESTS_API"))
	if base == "" {
		base = cfg.APIBase
	}
	c := apiclient.NewClient(base, true)
	c.Source = "overlay"
	timeout := 1500 * time.Millisecond
	if !strings.Contains(c.Base, "127.0.0.1") && !strings.Contains(c.Base, "localhost") {
		// LAN / remote: body read can stall briefly; 3s was flashing the HUD.
		timeout = 5 * time.Second
	}
	if t := strings.TrimSpace(os.Getenv("QUESTS_API_TIMEOUT")); t != "" {
		if d, err := time.ParseDuration(t + "s"); err == nil {
			timeout = d
		}
	}
	c.HTTP.Timeout = timeout
	return &Client{Client: c}
}

// Quest is the HUD-relevant slice of /api/quests JSON.
type Quest struct {
	ID              int64   `json:"id"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`
	Pinned          bool    `json:"pinned"`
	OverlayHidden   bool    `json:"overlay_hidden"`
	DeadlineAt      *string `json:"deadline_at"`
	DurationSeconds *int    `json:"duration_seconds"`
	CategorySlug    string  `json:"category_slug"`
	Urgent          bool    `json:"urgent"`
	Steps           []Step  `json:"steps"`
}

// Step is an open/closed quest step.
type Step struct {
	Title           string `json:"title"`
	Done            bool   `json:"done"`
	ProgressCurrent int    `json:"progress_current"`
	ProgressTotal   int    `json:"progress_total"`
}

// Category from /api/categories.
type Category struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

// FetchQuests loads all quests for HUD splitting.
func (c *Client) FetchQuests() ([]Quest, error) {
	raw, err := c.Get("/api/quests", nil)
	if err != nil {
		return nil, err
	}
	var out []Quest
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// FetchCategories loads category lanes for the HUD.
func (c *Client) FetchCategories() ([]Category, error) {
	raw, err := c.Get("/api/categories", nil)
	if err != nil {
		return nil, err
	}
	var out []Category
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// PostHeartbeat pings /api/health/heartbeat so the SPA shows the HUD chip.
func (c *Client) PostHeartbeat(detail string) {
	_, _ = c.Post("/api/health/heartbeat", nil, map[string]string{
		"component": "overlay",
		"detail":    detail,
	})
}

// FetchEvents returns revision and events since the given revision.
func (c *Client) FetchEvents(since int64) (revision int64, events []map[string]any, err error) {
	raw, err := c.Get("/api/events", map[string]string{"since": strconv.FormatInt(since, 10)})
	if err != nil {
		return since, nil, err
	}
	var payload struct {
		Revision int64            `json:"revision"`
		Events   []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return since, nil, err
	}
	if payload.Revision == 0 {
		payload.Revision = since
	}
	return payload.Revision, payload.Events, nil
}
