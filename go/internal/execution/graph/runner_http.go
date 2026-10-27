package graph

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// `http.runner` is `http.get` that the runner sends instead of the server: from the machine of the person who owns it, so it
// reaches what the server may not (a local network, a host the administrator did not open) and needs no policy of the server.
// The node is a Code node in the protocol: a job that carries the request, answered by the runner's text. What the owner limits
// is in the node: `allow` (hosts the request and its redirects may touch; empty means no limit), the time and the size.

// RunnerHTTPMarker is the first line of the command of such a job; a runner that knows it sends the request itself, an older one
// would run the rest as a shell command and fail loudly.
const RunnerHTTPMarker = "#!quests-http"

const (
	runnerHTTPType        = "http.runner"
	defaultRunnerHTTPSecs = 20
	maxRunnerHTTPSecs     = 300
	defaultRunnerHTTPSize = 256 << 10
	maxRunnerHTTPSize     = 400 << 10 // a runner returns the tail of 512 KiB of output, a document must fit with its wrapper
)

type runnerHTTPParams struct {
	httpParams
	Scheme     string            `json:"scheme,omitempty"` // https (default) or http
	Headers    map[string]string `json:"headers,omitempty"`
	Allow      []string          `json:"allow,omitempty"`
	Timeout    int               `json:"timeout_sec,omitempty"`
	MaxBytes   int               `json:"max_bytes,omitempty"`
	NoRedirect bool              `json:"no_redirect,omitempty"`
}

func runnerHTTPPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p runnerHTTPParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	in, _, err := httpPorts(rawJSON(p.httpParams))
	if err != nil {
		return nil, nil, err
	}
	switch p.Scheme {
	case "", "http", "https":
	default:
		return nil, nil, errors.New(`параметр "scheme": http или https`)
	}
	if len(p.Headers) > 8 {
		return nil, nil, errors.New("заголовков не больше 8")
	}
	for k, v := range p.Headers {
		if !headerNameRe.MatchString(k) || forbiddenHeaders[strings.ToLower(k)] || len(v) > 200 {
			return nil, nil, fmt.Errorf("заголовок %q: обычное имя, не служебное, значение до 200 символов", k)
		}
	}
	if len(p.Allow) > 16 {
		return nil, nil, errors.New("в списке allow не больше 16 узлов")
	}
	for _, h := range p.Allow {
		if !validAllow(h) {
			return nil, nil, fmt.Errorf("allow: %q не имя узла (a.example.org или *.example.org)", h)
		}
	}
	if p.Timeout < 0 || p.Timeout > maxRunnerHTTPSecs {
		return nil, nil, fmt.Errorf("timeout_sec: от 1 до %d", maxRunnerHTTPSecs)
	}
	if p.MaxBytes < 0 || p.MaxBytes > maxRunnerHTTPSize {
		return nil, nil, fmt.Errorf("max_bytes: до %d", maxRunnerHTTPSize)
	}
	return in, []Port{req("body", TDoc), req("text", TStr), req("status", TInt), req("failed", TDoc)}, nil
}

func validAllow(h string) bool {
	return hostRe.MatchString(strings.TrimPrefix(strings.ToLower(h), "*."))
}

func rawJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// RunnerHTTPJob is what the runner reads after the marker line.
type RunnerHTTPJob struct {
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"`
	Auth       *FetchAuth        `json:"auth,omitempty"`
	Allow      []string          `json:"allow,omitempty"`
	TimeoutSec int               `json:"timeout_sec"`
	MaxBytes   int               `json:"max_bytes"`
	NoRedirect bool              `json:"no_redirect,omitempty"`
}

// RunnerHTTPAnswer is what the runner prints on a clean exit.
type RunnerHTTPAnswer struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Cut    bool   `json:"cut,omitempty"`
}

// runnerHTTPRequest builds the job of a ready `http.runner` node.
func (r *Run) runnerHTTPRequest(rs *resolved, ins map[string][]Token) (*CodeRequest, error) {
	var p runnerHTTPParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	scheme := p.Scheme
	if scheme == "" {
		scheme = "https"
	}
	u, err := buildFetchURL(scheme, p.httpParams, ins)
	if err != nil {
		return nil, err
	}
	job := RunnerHTTPJob{URL: u, Headers: p.Headers, Auth: p.Auth, Allow: p.Allow, TimeoutSec: p.Timeout, MaxBytes: p.MaxBytes, NoRedirect: p.NoRedirect}
	if job.TimeoutSec == 0 {
		job.TimeoutSec = defaultRunnerHTTPSecs
	}
	if job.MaxBytes == 0 {
		job.MaxBytes = defaultRunnerHTTPSize
	}
	c := &CodeRequest{Node: rs.node.ID, Text: RunnerHTTPMarker + "\n" + string(rawJSON(job)), Env: map[string]string{}}
	if toks := ins["secrets"]; len(toks) == 1 && !toks[0].Exit {
		c.Secrets = true
		if sec, ok := toks[0].Value.(Secrets); ok {
			c.SecretsOf = sec.Of
		}
	}
	return c, nil
}

// finishRunnerHTTP turns how the job ended into the outputs of the node: a clean exit carries the answer, anything else is
// `failed` as the runner reported it.
func finishRunnerHTTP(o CodeOutcome) map[string]Token {
	if o.Port != "stdout" {
		return map[string]Token{"failed": {Type: TDoc, Value: json.RawMessage(o.Text)}}
	}
	var a RunnerHTTPAnswer
	if err := json.Unmarshal([]byte(strings.TrimSpace(o.Text)), &a); err != nil {
		return map[string]Token{"failed": failedDoc(FetchError, "раннер ответил не так, как ждёт узел (старая версия раннера?)", 0)}
	}
	if a.Cut {
		return map[string]Token{"failed": failedDoc(FetchTooLarge, "ответ больше допустимого размера", a.Status)}
	}
	return answerTokens(a.Status, []byte(a.Body))
}
