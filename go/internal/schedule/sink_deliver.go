package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
	"github.com/quests-inc/quests/go/internal/platform/egress"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

const deliverTimeout = 15 * time.Second

func (r *CheckRunner) sinkHTTP(ctx context.Context, n graph.Node, in map[string]graph.Token) error {
	var p struct {
		URL    string `json:"url"`
		Method string `json:"method"`
	}
	_ = json.Unmarshal(n.Params, &p)
	method := strings.ToUpper(strings.TrimSpace(p.Method))
	if method == "" {
		method = http.MethodPost
	}
	body, err := bodyBytes(in["body"])
	if err != nil {
		return err
	}
	res, err := r.egressDo(ctx, egress.Request{
		Method: method, URL: strings.TrimSpace(p.URL), Body: body,
		Header: map[string]string{"Content-Type": "application/json; charset=utf-8"},
	})
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	if res.Status >= 400 {
		return fmt.Errorf("http: ответ %d", res.Status)
	}
	return nil
}

// egressDo is the only way the server's scripts reach the outside: the administrator's policy, the workspace's grant, the log.
func (r *CheckRunner) egressDo(ctx context.Context, req egress.Request) (*egress.Result, error) {
	if r.Egress == nil || !r.Egress.Policy().Enabled() {
		return nil, &egress.Denied{Reason: "администратор сервера не разрешил исходящие запросы"}
	}
	ctx = context.WithoutCancel(ctx)
	if !r.Store.Execution.EgressEnabled(ctx) {
		return nil, &egress.Denied{Reason: "для этого пространства исходящие запросы не включены администратором"}
	}
	req.Workspace = wsctx.From(ctx)
	return r.Egress.Do(ctx, req)
}

func (r *CheckRunner) sinkSocket(_ context.Context, n graph.Node, in map[string]graph.Token) error {
	var p struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(n.Params, &p)
	path := strings.TrimSpace(p.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("socket: нужен абсолютный путь")
	}
	body, err := bodyBytes(in["body"])
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", path, deliverTimeout)
	if err != nil {
		return fmt.Errorf("socket: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(deliverTimeout))
	if !bytes.HasSuffix(body, []byte("\n")) {
		body = append(body, '\n')
	}
	_, err = conn.Write(body)
	return err
}

func bodyBytes(v any) ([]byte, error) {
	switch t := v.(type) {
	case nil:
		return []byte("null"), nil
	case string:
		return []byte(t), nil
	case json.RawMessage:
		return t, nil
	case []byte:
		return t, nil
	default:
		return json.Marshal(t)
	}
}
