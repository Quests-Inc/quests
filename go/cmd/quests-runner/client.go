package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	errUnauthorized = errors.New("server rejected the runner token (revoked?)")
	errLeaseLost    = errors.New("lease lost")
)

type client struct {
	base  string
	token string
	http  *http.Client
}

func newClient(base, token string) *client {
	return &client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Timeout: 45 * time.Second}}
}

// job is what the server hands out.
type job struct {
	ID             int64             `json:"id"`
	LeaseID        string            `json:"lease_id"`
	LeaseSeconds   int               `json:"lease_seconds"`
	Kind           string            `json:"kind"`
	Title          string            `json:"title"`
	Command        string            `json:"command"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	AttemptNo      int               `json:"attempt_no"`
	QuestID        *int64            `json:"quest_id"`
	StepID         *int64            `json:"step_id"`
	TemplateID     *int64            `json:"template_id"`
	NoteID         *int64            `json:"note_id"`
	Env            map[string]string `json:"env"`
}

// result is what a finished job reports.
type result struct {
	JobID    int64  `json:"job_id"`
	LeaseID  string `json:"lease_id"`
	Exit     int    `json:"exit"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	TimedOut bool   `json:"timed_out"`
}

func (c *client) do(ctx context.Context, method, path string, in, out any) (int, error) {
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return resp.StatusCode, errUnauthorized
	case resp.StatusCode == http.StatusConflict:
		return resp.StatusCode, errLeaseLost
	case resp.StatusCode >= 400:
		return resp.StatusCode, fmt.Errorf("%s %s: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (c *client) register(ctx context.Context, regToken, host, version string) (label, token string, err error) {
	var out struct {
		Token string `json:"token"`
		Label string `json:"label"`
	}
	_, err = c.do(ctx, "POST", "/api/runner/register", map[string]string{"registration_token": regToken, "hostname": host, "version": version}, &out)
	return out.Label, out.Token, err
}

// poll waits up to wait for a job; nil, nil means none came.
func (c *client) poll(ctx context.Context, wait time.Duration) (*job, error) {
	var j job
	code, err := c.do(ctx, "GET", "/api/runner/jobs?wait="+url.QueryEscape(fmt.Sprint(int(wait/time.Second))), nil, &j)
	if err != nil {
		return nil, err
	}
	if code == http.StatusNoContent || j.ID == 0 {
		return nil, nil
	}
	return &j, nil
}

type heartbeatIn struct {
	LeaseID         string `json:"lease_id"`
	ProgressCurrent *int   `json:"progress_current,omitempty"`
	ProgressTotal   *int   `json:"progress_total,omitempty"`
	LogTail         string `json:"log_tail"`
}

func (c *client) heartbeat(ctx context.Context, id int64, in heartbeatIn) (cancel bool, err error) {
	var out struct {
		Cancel bool `json:"cancel"`
	}
	_, err = c.do(ctx, "POST", fmt.Sprintf("/api/runner/jobs/%d/heartbeat", id), in, &out)
	return out.Cancel, err
}

func (c *client) complete(ctx context.Context, r result) error {
	_, err := c.do(ctx, "POST", fmt.Sprintf("/api/runner/jobs/%d/complete", r.JobID), r, nil)
	return err
}
