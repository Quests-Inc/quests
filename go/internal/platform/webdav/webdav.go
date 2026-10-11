// Package webdav is a minimal client for the file server that backs
// attachments. Quests keeps only metadata; the bytes live on WebDAV, and the
// credentials never leave the server — clients fetch through the API instead.
package webdav

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotFound = errors.New("webdav: not found")

type Client struct {
	BaseURL string
	User    string
	Pass    string
	HTTP    *http.Client
}

func New(baseURL, user, pass string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		User:    user,
		Pass:    pass,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Configured reports whether attachments can be stored at all.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" }

// Ping is a cheap liveness check (HEAD, then OPTIONS if the host rejects HEAD).
// 404/405 still count as up — the server spoke. 401/403/5xx and network
// errors do not: attachments would fail the same way.
func (c *Client) Ping(ctx context.Context) error {
	if !c.Configured() {
		return fmt.Errorf("webdav: not configured")
	}
	for _, method := range []string{http.MethodHead, http.MethodOptions} {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/", nil)
		if err != nil {
			return err
		}
		resp, err := c.do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusMethodNotAllowed:
			continue
		case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
			return fmt.Errorf("webdav: ping %s", resp.Status)
		case resp.StatusCode >= 500:
			return fmt.Errorf("webdav: ping %s", resp.Status)
		default:
			return nil
		}
	}
	return nil
}

// urlFor escapes each path segment separately: a filename may contain spaces
// or non-ASCII, but the slashes are structure, not content.
func (c *Client) urlFor(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return c.BaseURL + "/" + strings.Join(parts, "/")
}

func (c *Client) do(req *http.Request) (*http.Response, error) {
	if c.User != "" || c.Pass != "" {
		req.SetBasicAuth(c.User, c.Pass)
	}
	return c.HTTP.Do(req)
}

// MkcolAll creates every missing collection along dir. WebDAV MKCOL only
// creates one level at a time, and 405 means "already there".
func (c *Client) MkcolAll(ctx context.Context, dir string) error {
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		req, err := http.NewRequestWithContext(ctx, "MKCOL", c.urlFor(prefix), nil)
		if err != nil {
			return err
		}
		resp, err := c.do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		switch resp.StatusCode {
		case http.StatusCreated, http.StatusOK, http.StatusMethodNotAllowed, http.StatusConflict:
			// created, already exists, or parent missing — the next PUT tells us
		default:
			if resp.StatusCode >= 400 {
				return fmt.Errorf("webdav: MKCOL %s: %s", prefix, resp.Status)
			}
		}
	}
	return nil
}

func (c *Client) Put(ctx context.Context, path, contentType string, body io.Reader, size int64) error {
	if i := strings.LastIndex(strings.Trim(path, "/"), "/"); i > 0 {
		if err := c.MkcolAll(ctx, strings.Trim(path, "/")[:i]); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.urlFor(path), body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if size > 0 {
		req.ContentLength = size
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav: PUT %s: %s", path, resp.Status)
	}
	return nil
}

// Get returns the body; the caller closes it.
func (c *Client) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.urlFor(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("webdav: GET %s: %s", path, resp.Status)
	}
	return resp.Body, nil
}

func (c *Client) Delete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.urlFor(path), nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webdav: DELETE %s: %s", path, resp.Status)
	}
	return nil
}

type Info struct {
	Size         int64
	LastModified time.Time
}

// Stat is a HEAD, not a PROPFIND: everything the "source changed" marker needs
// (Last-Modified, size) comes back in plain HTTP headers.
func (c *Client) Stat(ctx context.Context, path string) (Info, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.urlFor(path), nil)
	if err != nil {
		return Info{}, err
	}
	resp, err := c.do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Info{}, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return Info{}, fmt.Errorf("webdav: HEAD %s: %s", path, resp.Status)
	}
	info := Info{Size: resp.ContentLength}
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			info.LastModified = t.UTC()
		}
	}
	return info, nil
}
