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

// httpMarker is the first line of the command of an `http.runner` node (graph.RunnerHTTPMarker): the request is sent by this
// process, from this machine, under the limits the node names. Nothing else limits it: the runner belongs to the person who
// runs it.
const httpMarker = "#!quests-http"

type httpJob struct {
	URL        string                                   `json:"url"`
	Headers    map[string]string                        `json:"headers"`
	Auth       *struct{ Header, Secret, Prefix string } `json:"auth"`
	Allow      []string                                 `json:"allow"`
	TimeoutSec int                                      `json:"timeout_sec"`
	MaxBytes   int                                      `json:"max_bytes"`
	NoRedirect bool                                     `json:"no_redirect"`
}

type httpAnswer struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Cut    bool   `json:"cut,omitempty"`
}

func isHTTPJob(command string) bool { return strings.HasPrefix(command, httpMarker+"\n") }

// allowedHost says whether host fits the node's list (`a.example.org`, or `*.example.org` for a subdomain); an empty list is no limit.
func allowedHost(allow []string, host string) bool {
	if len(allow) == 0 {
		return true
	}
	host = strings.ToLower(host)
	for _, a := range allow {
		a = strings.ToLower(a)
		if rest, ok := strings.CutPrefix(a, "*."); ok {
			if strings.HasSuffix(host, "."+rest) {
				return true
			}
		} else if host == a {
			return true
		}
	}
	return false
}

// runHTTPJob sends the request and prints the answer as a document on stdout. A status of 4xx/5xx is an answer, not a failure of
// the job; only a request that got none exits non-zero.
func runHTTPJob(ctx context.Context, command string, env map[string]string, timeout time.Duration, stdout, stderr io.Writer) (exit int, timedOut, canceled bool) {
	var j httpJob
	if err := json.Unmarshal([]byte(strings.TrimPrefix(command, httpMarker+"\n")), &j); err != nil {
		fmt.Fprintf(stderr, "запрос не разобрать: %v\n", err)
		return 2, false, false
	}
	if j.TimeoutSec > 0 && time.Duration(j.TimeoutSec)*time.Second < timeout {
		timeout = time.Duration(j.TimeoutSec) * time.Second
	}
	if j.MaxBytes <= 0 {
		j.MaxBytes = 256 << 10
	}
	u, err := url.Parse(j.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		fmt.Fprintf(stderr, "адрес не годится: %q\n", j.URL)
		return 2, false, false
	}
	if !allowedHost(j.Allow, u.Hostname()) {
		fmt.Fprintf(stderr, "узел %s не в списке разрешённых узла\n", u.Hostname())
		return 3, false, false
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2, false, false
	}
	req.Header.Set("Accept", "application/json, text/plain;q=0.9, */*;q=0.1")
	req.Header.Set("User-Agent", "quests-runner/http")
	for k, v := range j.Headers {
		req.Header.Set(k, v)
	}
	if a := j.Auth; a != nil {
		secret, ok := env[a.Secret]
		if !ok {
			fmt.Fprintf(stderr, "секрет %s не пришёл с задачей\n", a.Secret)
			return 4, false, false
		}
		req.Header.Set(a.Header, a.Prefix+secret)
	}
	hc := &http.Client{CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if j.NoRedirect {
			return http.ErrUseLastResponse
		}
		if len(via) >= 5 {
			return errors.New("слишком много редиректов")
		}
		if !allowedHost(j.Allow, r.URL.Hostname()) {
			return fmt.Errorf("редирект на %s вне списка разрешённых", r.URL.Hostname())
		}
		if a := j.Auth; a != nil && r.URL.Host != via[0].URL.Host {
			r.Header.Del(a.Header) // a secret does not follow a redirect to another host
		}
		return nil
	}}
	resp, err := hc.Do(req)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			fmt.Fprintln(stderr, "ответ не пришёл вовремя")
			return 124, true, false
		case errors.Is(err, context.Canceled):
			return 130, false, true
		}
		fmt.Fprintln(stderr, err)
		return 1, false, false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(j.MaxBytes)+1))
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(stderr, "ответ не пришёл вовремя")
			return 124, true, false
		}
		fmt.Fprintln(stderr, err)
		return 1, false, false
	}
	ans := httpAnswer{Status: resp.StatusCode}
	if len(body) > j.MaxBytes {
		ans.Cut = true
	} else {
		ans.Body = string(bytes.ToValidUTF8(body, []byte("�")))
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(ans)
	if out.Len() > outputKeep-1024 { // the server reads the tail of the output: a document that does not fit would be torn
		out.Reset()
		_ = enc.Encode(httpAnswer{Status: resp.StatusCode, Cut: true})
	}
	_, _ = stdout.Write(out.Bytes())
	return 0, false, false
}
