package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The nodes that read the outside (`http.get`) and the nodes that take what they read apart (`json.get`). Fetching is I/O, so
// the node does not do it: it describes the request and the host that runs the graph supplies the Fetcher, which is where the
// administrator's policy lives (ADR-0004). A graph run with no Fetcher cannot go out.

// Failure kinds of a fetch, as the `failed` document names them.
const (
	FetchDenied     = "denied"
	FetchTimeout    = "timeout"
	FetchStatus     = "status"
	FetchTooLarge   = "too_large"
	FetchBadContent = "bad_content"
	FetchError      = "error"
)

// FetchAuth puts a secret into one request header. The secret is read where the request is sent and goes nowhere else.
type FetchAuth struct {
	Header string `json:"header"`
	Secret string `json:"secret"`
	Prefix string `json:"prefix,omitempty"`
}

// FetchRequest is what a node asks the host to send: a GET of an absolute https URL.
type FetchRequest struct {
	URL  string
	Auth *FetchAuth
	// SecretsOf is the entity whose secrets Auth names; zero when the node has no `secrets` input wired.
	SecretsOf Ref
}

// FetchResult is a received answer.
type FetchResult struct {
	Status int
	Body   []byte
}

// FetchFailure is a request that did not give an answer (or gave an error one): the kind says which.
type FetchFailure struct {
	Kind    string
	Message string
	Status  int
}

func (e *FetchFailure) Error() string { return e.Message }

// Fetcher sends a request. Errors that are not *FetchFailure count as FetchError.
type Fetcher func(ctx context.Context, req FetchRequest) (FetchResult, error)

var (
	hostRe       = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*(:[0-9]{1,5})?$`)
	headerNameRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]{0,39}$`)
	secretNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
	jsonSegRe    = regexp.MustCompile(`^[A-Za-z0-9_\-]{1,64}$`)
)

const (
	maxFetchPath  = 300
	maxFetchQuery = 16
	maxJSONPath   = 200
)

// forbiddenHeaders are the ones a node may not set: they belong to the transport or would let a secret be sent as something else.
var forbiddenHeaders = map[string]bool{
	"host": true, "content-length": true, "transfer-encoding": true, "connection": true, "upgrade": true, "te": true,
	"cookie": true, "set-cookie": true, "proxy-authorization": true, "user-agent": true, "accept-encoding": true,
}

type httpParams struct {
	Host  string            `json:"host"`
	Path  string            `json:"path,omitempty"`
	Query map[string]string `json:"query,omitempty"`
	Auth  *FetchAuth        `json:"auth,omitempty"`
}

func httpPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p httpParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	p.Host = strings.ToLower(strings.TrimSpace(p.Host))
	if !hostRe.MatchString(p.Host) {
		return nil, nil, fmt.Errorf("параметр %q: имя узла без схемы и пути, например api.example.org", "host")
	}
	if len(p.Path) > maxFetchPath || strings.ContainsAny(p.Path, "?#\r\n ") || (p.Path != "" && !strings.HasPrefix(p.Path, "/")) {
		return nil, nil, fmt.Errorf("параметр %q: путь с «/» в начале, без «?», «#» и пробелов, до %d символов", "path", maxFetchPath)
	}
	if len(p.Query) > maxFetchQuery {
		return nil, nil, fmt.Errorf("в запросе не больше %d параметров", maxFetchQuery)
	}
	for k, v := range p.Query {
		if k == "" || len(k) > 64 || len(v) > 200 {
			return nil, nil, fmt.Errorf("параметр запроса %q: имя до 64, значение до 200 символов", k)
		}
	}
	in := []Port{opt("path", TStr), opt("query", TDoc), opt("trigger", TAny)}
	if p.Auth != nil {
		if !headerNameRe.MatchString(p.Auth.Header) || forbiddenHeaders[strings.ToLower(p.Auth.Header)] {
			return nil, nil, errors.New("auth.header: имя заголовка (Authorization, X-Api-Key…), не служебное")
		}
		if !secretNameRe.MatchString(p.Auth.Secret) || len(p.Auth.Prefix) > 20 {
			return nil, nil, errors.New("auth.secret: имя секрета; auth.prefix не длиннее 20 символов")
		}
		in = append(in, req("secrets", TSecret))
	}
	return in, []Port{req("body", TDoc), req("text", TStr), req("status", TInt), req("failed", TDoc)}, nil
}

// runHTTP builds the URL from what the owner fixed (host, path, query) and from what arrived (path piece, query values), asks the
// host to send it and puts the answer on the outputs. A failure is data on `failed`, like a Code node's.
func (r *Run) runHTTP(ctx context.Context, rs *resolved, ins map[string][]Token) (map[string]Token, error) {
	var p httpParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	if r.Fetch == nil {
		return nil, errors.New("исходящие запросы здесь недоступны")
	}
	p.Host = strings.ToLower(strings.TrimSpace(p.Host))
	path := p.Path
	piece := ""
	if toks := ins["path"]; len(toks) == 1 && !toks[0].Exit {
		piece, _ = toks[0].Value.(string)
	}
	if strings.Contains(path, "{path}") {
		path = strings.ReplaceAll(path, "{path}", url.PathEscape(piece))
	} else if piece != "" {
		path = strings.TrimSuffix(path, "/") + "/" + url.PathEscape(piece)
	}
	q := url.Values{}
	for k, v := range p.Query {
		q.Set(k, v)
	}
	if toks := ins["query"]; len(toks) == 1 && !toks[0].Exit {
		if raw, ok := toks[0].Value.(json.RawMessage); ok {
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				return nil, errors.New("query: нужен JSON-объект со скалярами")
			}
			for k, v := range m {
				switch s := v.(type) {
				case string:
					q.Set(k, s)
				case float64, bool:
					q.Set(k, fmt.Sprint(s))
				default:
					return nil, fmt.Errorf("query.%s: нужен скаляр", k)
				}
			}
		}
	}
	if len(q) > maxFetchQuery {
		return nil, fmt.Errorf("в запросе не больше %d параметров", maxFetchQuery)
	}
	u := "https://" + p.Host + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	fr := FetchRequest{URL: u, Auth: p.Auth}
	if toks := ins["secrets"]; len(toks) == 1 && !toks[0].Exit {
		if sec, ok := toks[0].Value.(Secrets); ok {
			fr.SecretsOf = sec.Of
		}
	}
	res, err := r.Fetch(ctx, fr)
	if err != nil {
		var ff *FetchFailure
		if !errors.As(err, &ff) {
			ff = &FetchFailure{Kind: FetchError, Message: err.Error()}
		}
		return map[string]Token{"failed": failedDoc(ff.Kind, ff.Message, ff.Status)}, nil
	}
	out := map[string]Token{"status": {Type: TInt, Value: int64(res.Status)}}
	if res.Status >= 400 {
		out["failed"] = failedDoc(FetchStatus, "ответ "+strconv.Itoa(res.Status), res.Status)
		return out, nil
	}
	if !utf8.Valid(res.Body) {
		return map[string]Token{"failed": failedDoc(FetchBadContent, "ответ не текст", res.Status)}, nil
	}
	out["text"] = Token{Type: TStr, Value: string(res.Body)}
	if trimmed := strings.TrimSpace(string(res.Body)); trimmed != "" && (trimmed[0] == '{' || trimmed[0] == '[') && json.Valid([]byte(trimmed)) {
		out["body"] = Token{Type: TDoc, Value: json.RawMessage(trimmed)}
	}
	return out, nil
}

func failedDoc(kind, message string, status int) Token {
	m := map[string]any{"kind": kind, "message": message}
	if status > 0 {
		m["status"] = status
	}
	b, _ := json.Marshal(m)
	return Token{Type: TDoc, Value: json.RawMessage(b)}
}

type jsonGetParams struct {
	Path string   `json:"path"`
	Type PortType `json:"type"`
}

func jsonGetPorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p jsonGetParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if _, err := splitJSONPath(p.Path); err != nil {
		return nil, nil, err
	}
	if !p.Type.value() {
		return nil, nil, fmt.Errorf("параметр %q: int, float, str, bool или doc", "type")
	}
	return []Port{req("in", TValue)}, []Port{req("value", p.Type), req("bad", TStr)}, nil
}

func splitJSONPath(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" || len(path) > maxJSONPath {
		return nil, fmt.Errorf("параметр %q: путь вида current.temperature_2m или items.0.name, до %d символов", "path", maxJSONPath)
	}
	segs := strings.Split(path, ".")
	for _, s := range segs {
		if !jsonSegRe.MatchString(s) {
			return nil, fmt.Errorf("параметр %q: в пути только буквы, цифры, «_», «-» и точки между частями", "path")
		}
	}
	return segs, nil
}

// runJSONGet takes one value out of a JSON document by a dotted path (an object key or an array index) and gives it as the type
// asked. A path that is not there, or a value of another type, leaves `value` without a token and says why on `bad`.
func runJSONGet(rs *resolved, in Token) (map[string]Token, error) {
	var p jsonGetParams
	if err := strictParams(rs.node.Params, &p); err != nil {
		return nil, err
	}
	segs, err := splitJSONPath(p.Path)
	if err != nil {
		return nil, err
	}
	bad := func(msg string) (map[string]Token, error) {
		return map[string]Token{"bad": {Type: TStr, Value: msg}}, nil
	}
	var text []byte
	switch v := in.Value.(type) {
	case json.RawMessage:
		text = v
	case string:
		text = []byte(v)
	default:
		return bad("на входе не документ")
	}
	var cur any
	if err := json.Unmarshal(text, &cur); err != nil {
		return bad("на входе не JSON")
	}
	for _, s := range segs {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[s]
			if !ok {
				return bad("нет ключа " + s)
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(s)
			if err != nil || i < 0 || i >= len(node) {
				return bad("нет элемента " + s)
			}
			cur = node[i]
		default:
			return bad("в «" + s + "» нечего искать")
		}
	}
	val := func(t PortType, v any) (map[string]Token, error) {
		return map[string]Token{"value": {Type: t, Value: v}}, nil
	}
	switch p.Type {
	case TStr:
		if s, ok := cur.(string); ok {
			return val(TStr, s)
		}
	case TBool:
		if b, ok := cur.(bool); ok {
			return val(TBool, b)
		}
	case TFloat:
		if f, ok := cur.(float64); ok {
			return val(TFloat, f)
		}
	case TInt:
		if f, ok := cur.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			return val(TInt, int64(f))
		}
	case TDoc:
		b, _ := json.Marshal(cur)
		return val(TDoc, json.RawMessage(b))
	}
	return bad("значение другого типа, нужен " + string(p.Type))
}

// UsesNetwork reports whether the graph holds a node that goes out, in the sorted order of its types.
func (g Graph) UsesNetwork(reg Registry) []string {
	seen := map[string]bool{}
	for _, n := range g.Nodes {
		if t, ok := reg[n.Type]; ok && t.Network {
			seen[n.Type] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// EventFamilies are the kinds of event `source.event` can be set to read; `any` gives the common ports only.
var EventFamilies = map[string]bool{"any": true, "quest": true, "step": true, "note": true, "value": true, "attachment": true}

// EventSourceParams are the parameters of `source.event`: which family of events the ports are shaped for (a hook script knows
// what its hook waits for), and, for a variable, the kind of owner the `ref` output points at.
type EventSourceParams struct {
	Family string `json:"family,omitempty"`
	Owner  string `json:"owner,omitempty"`
}

var ownerRefTypes = map[string]PortType{"quest": TRefQuest, "step": TRefStep, "note": TRefNote, "template": TRefTemplate}

func eventSourcePorts(raw json.RawMessage) ([]Port, []Port, error) {
	var p EventSourceParams
	if err := strictParams(raw, &p); err != nil {
		return nil, nil, err
	}
	if p.Family == "" {
		p.Family = "any"
	}
	if !EventFamilies[p.Family] {
		return nil, nil, fmt.Errorf("параметр %q: any, quest, step, note, value или attachment", "family")
	}
	out := []Port{req("kind", TStr), req("title", TStr), req("detail", TStr), req("event", TDoc)}
	switch p.Family {
	case "quest":
		out = append(out, req("ref", TRefQuest), req("secrets", TSecret), req("quest_id", TInt))
	case "step":
		out = append(out, req("ref", TRefStep), req("secrets", TSecret), req("step_id", TInt), req("quest_id", TInt), req("step_title", TStr))
	case "note":
		out = append(out, req("ref", TRefNote), req("secrets", TSecret), req("note_id", TInt))
	case "value":
		if p.Owner == "" {
			p.Owner = "note"
		}
		t, ok := ownerRefTypes[p.Owner]
		if !ok {
			return nil, nil, fmt.Errorf("параметр %q: quest, step, note или template", "owner")
		}
		out = append(out, req("ref", t), req("secrets", TSecret), req("owner_kind", TStr), req("owner_id", TInt), req("name", TStr), req("type", TStr), req("value", TDoc))
	case "attachment":
		out = append(out, req("attachment_id", TInt), req("owner_type", TStr), req("owner_id", TInt))
	}
	return nil, out, nil
}
