package egress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Denied is a request the policy refused: the administrator has not opened that door. Kind names the reason for a script.
type Denied struct{ Reason string }

func (d *Denied) Error() string {
	return "запрещено политикой сервера: " + d.Reason
}

var (
	// ErrTooLarge: the answer is longer than the limit (after it was unpacked).
	ErrTooLarge = errors.New("ответ больше допустимого размера")
	// ErrBusy: too many requests at once.
	ErrBusy = errors.New("слишком много исходящих запросов сразу")
	// ErrRate: the workspace used its hour.
	ErrRate = errors.New("исчерпан лимит исходящих запросов в час")
)

// Entry is one line of the log of outgoing requests.
type Entry struct {
	Workspace int64
	ScriptID  int64
	Host      string
	Method    string
	Status    int
	Bytes     int64
	Millis    int64
	Result    string
}

// Request is what a node wants sent. URL must be absolute; the host in it is judged by the policy, whoever wrote it.
type Request struct {
	Workspace int64
	ScriptID  int64
	Method    string
	URL       string
	Body      []byte
	Header    map[string]string
}

// Result is a received answer. The headers do not leave this package except what a caller needs.
type Result struct {
	Status      int
	Body        []byte
	ContentType string
}

// TextLike says whether the answer is something a script may read as text or JSON.
func (r *Result) TextLike() bool {
	t, _, _ := strings.Cut(strings.ToLower(r.ContentType), ";")
	t = strings.TrimSpace(t)
	return t == "" || strings.HasPrefix(t, "text/") || t == "application/json" || t == "application/xml" ||
		strings.HasSuffix(t, "+json") || strings.HasSuffix(t, "+xml")
}

// Client sends the requests of scripts under a Policy.
type Client struct {
	policy Policy
	log    func(Entry)
	http   *http.Client
	sem    chan struct{}

	mu   sync.Mutex
	used map[int64][]time.Time
	now  func() time.Time
}

// New builds a client. log (may be nil) is told about every attempt, refused ones included.
func New(p Policy, log func(Entry)) *Client {
	c := &Client{policy: p, log: log, used: map[int64][]time.Time{}, now: time.Now}
	if p.Concurrency < 1 {
		p.Concurrency = DefaultConcurrency
	}
	c.sem = make(chan struct{}, p.Concurrency)
	proxyAddrs := proxyAddresses(p.Proxy)
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
		// Control runs for every address the connection is about to use, after the name was resolved: a name that answers with a
		// public address now and an internal one a moment later gets the second refused too, and so does a redirect.
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return &Denied{Reason: "адрес соединения"}
			}
			a, err := netip.ParseAddr(host)
			if err == nil && proxyAddrs[address] {
				return nil // the administrator's own proxy, which may well be on this machine
			}
			if err != nil || Blocked(a, p.AllowNets) {
				return &Denied{Reason: "внутренний адрес"}
			}
			return nil
		},
	}
	tr := &http.Transport{
		Proxy:                 proxyFunc(p.Proxy), // the administrator's, or none: never the environment's
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: p.Timeout,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     true,
	}
	c.http = &http.Client{
		Transport: tr,
		Timeout:   p.Timeout,
		Jar:       nil,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return &Denied{Reason: "слишком много редиректов"}
			}
			_, err := c.check(req.URL)
			return err
		},
	}
	return c
}

// Policy returns the rules the client works under.
func (c *Client) Policy() Policy { return c.policy }

// check judges an URL before any connection: scheme, user info, host, port.
func (c *Client) check(u *url.URL) (string, error) {
	if !c.policy.Enabled() {
		return "", &Denied{Reason: "администратор сервера не разрешил исходящие запросы"}
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", &Denied{Reason: "только http и https"}
	}
	if u.User != nil {
		return "", &Denied{Reason: "адрес с логином и паролем"}
	}
	host, err := NormalizeHost(u.Hostname())
	if err != nil {
		return "", &Denied{Reason: err.Error()}
	}
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if ps := u.Port(); ps != "" {
		if port, err = strconv.Atoi(ps); err != nil {
			return "", &Denied{Reason: "порт"}
		}
	}
	if !c.policy.Ports[port] {
		return "", &Denied{Reason: "порт " + strconv.Itoa(port) + " не разрешён"}
	}
	if !c.policy.HostAllowed(host) {
		return "", &Denied{Reason: "узел " + host + " не в списке разрешённых"}
	}
	return host, nil
}

func (c *Client) reserve(ws int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	keep := c.used[ws][:0]
	for _, t := range c.used[ws] {
		if now.Sub(t) < time.Hour {
			keep = append(keep, t)
		}
	}
	if len(keep) >= c.policy.RatePerHour {
		c.used[ws] = keep
		return ErrRate
	}
	c.used[ws] = append(keep, now)
	return nil
}

// Do sends the request and reads the answer, within the policy.
func (c *Client) Do(ctx context.Context, in Request) (*Result, error) {
	start := time.Now()
	e := Entry{Workspace: in.Workspace, ScriptID: in.ScriptID, Method: strings.ToUpper(in.Method)}
	finish := func(res *Result, err error) (*Result, error) {
		e.Millis = time.Since(start).Milliseconds()
		if err == nil {
			e.Result, e.Status, e.Bytes = "ok", res.Status, int64(len(res.Body))
			if res.Status >= 400 {
				e.Result = "status"
			}
		} else {
			var d *Denied
			switch {
			case errors.As(err, &d):
				e.Result = "denied: " + d.Reason
			case errors.Is(err, ErrTooLarge):
				e.Result = "too_large"
			case errors.Is(err, context.DeadlineExceeded):
				e.Result = "timeout"
			case errors.Is(err, ErrRate), errors.Is(err, ErrBusy):
				e.Result = "limit"
			default:
				e.Result = "error"
			}
		}
		if c.log != nil {
			c.log(e)
		}
		return res, err
	}
	if e.Method == "" {
		e.Method = http.MethodGet
	}
	u, err := url.Parse(strings.TrimSpace(in.URL))
	if err != nil {
		return finish(nil, &Denied{Reason: "адрес не разобрать"})
	}
	host, err := c.check(u)
	if err != nil {
		e.Host = u.Hostname()
		return finish(nil, err)
	}
	e.Host = host
	if err := c.reserve(in.Workspace); err != nil {
		return finish(nil, err)
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	default:
		return finish(nil, ErrBusy)
	}
	ctx, cancel := context.WithTimeout(ctx, c.policy.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, e.Method, u.String(), bytes.NewReader(in.Body))
	if err != nil {
		return finish(nil, err)
	}
	for k, v := range in.Header {
		req.Header.Set(k, v)
	}
	req.Header.Set("User-Agent", "quests-egress/1")
	resp, err := c.http.Do(req)
	if err != nil {
		var d *Denied
		if errors.As(err, &d) {
			return finish(nil, d)
		}
		return finish(nil, fmt.Errorf("запрос: %w", unwrapURLError(err)))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.policy.MaxBody+1))
	if err != nil {
		return finish(nil, fmt.Errorf("ответ: %w", err))
	}
	if int64(len(body)) > c.policy.MaxBody {
		return finish(nil, ErrTooLarge)
	}
	return finish(&Result{Status: resp.StatusCode, Body: body, ContentType: resp.Header.Get("Content-Type")}, nil)
}

func unwrapURLError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func proxyFunc(u *url.URL) func(*http.Request) (*url.URL, error) {
	if u == nil {
		return nil
	}
	return http.ProxyURL(u)
}

// proxyAddresses are the addresses (ip:port) the connection to the administrator's proxy may use.
func proxyAddresses(u *url.URL) map[string]bool {
	out := map[string]bool{}
	if u == nil {
		return out
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return out
	}
	for _, ip := range ips {
		out[net.JoinHostPort(ip.String(), port)] = true
	}
	return out
}
