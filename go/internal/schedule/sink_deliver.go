package schedule

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/quests-inc/quests/go/internal/execution/graph"
)

const deliverTimeout = 15 * time.Second

func (r *CheckRunner) sinkHTTP(_ context.Context, n graph.Node, in map[string]graph.Token) error {
	var p struct {
		URL    string `json:"url"`
		Method string `json:"method"`
	}
	_ = json.Unmarshal(n.Params, &p)
	u, err := url.Parse(strings.TrimSpace(p.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("http: нужен http(s) URL")
	}
	if err := rejectPrivateHost(u.Hostname()); err != nil {
		return err
	}
	method := strings.ToUpper(strings.TrimSpace(p.Method))
	if method == "" {
		method = http.MethodPost
	}
	body, err := bodyBytes(in["body"])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), deliverTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http: ответ %d", resp.StatusCode)
	}
	return nil
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

func rejectPrivateHost(host string) error {
	host = strings.TrimSuffix(host, ".")
	if host == "" || strings.EqualFold(host, "localhost") {
		return fmt.Errorf("http: адрес недоступен")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("http: адрес недоступен")
		}
	}
	return nil
}
