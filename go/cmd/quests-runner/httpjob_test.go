package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func runJob(t *testing.T, job string, env map[string]string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	exit, _, _ := runHTTPJob(context.Background(), httpMarker+"\n"+job, env, 5*time.Second, &out, &errb)
	return exit, out.String(), errb.String()
}

func TestHTTPJobAnswersAndLimits(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Key")
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(`{"a":"<b>"}`))
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 2000)))
		case "/err":
			http.Error(w, "down", http.StatusServiceUnavailable)
		case "/redir":
			http.Redirect(w, r, "http://other.invalid/x", http.StatusFound)
		}
	}))
	defer srv.Close()
	j := func(path, extra string) string {
		return `{"url":"` + srv.URL + path + `","timeout_sec":3` + extra + `}`
	}

	exit, out, _ := runJob(t, j("/ok", `,"auth":{"Header":"X-Key","Secret":"KEY","Prefix":"k "}`), map[string]string{"KEY": "s3"})
	var a httpAnswer
	if exit != 0 || json.Unmarshal([]byte(out), &a) != nil || a.Status != 200 || a.Body != `{"a":"<b>"}` || gotKey != "k s3" {
		t.Fatalf("ok: exit=%d out=%q key=%q", exit, out, gotKey)
	}
	if exit, out, _ = runJob(t, j("/err", ""), nil); exit != 0 || !strings.Contains(out, `"status":503`) {
		t.Fatalf("an error status is an answer: %d %q", exit, out)
	}
	if exit, out, _ = runJob(t, j("/big", `,"max_bytes":100`), nil); exit != 0 || !strings.Contains(out, `"cut":true`) {
		t.Fatalf("too large: %d %q", exit, out)
	}
	if exit, _, msg := runJob(t, j("/ok", `,"allow":["example.org"]`), nil); exit != 3 || !strings.Contains(msg, "не в списке") {
		t.Fatalf("allow: %d %q", exit, msg)
	}
	if exit, _, msg := runJob(t, j("/redir", `,"allow":["127.0.0.1"]`), nil); exit == 0 || !strings.Contains(msg, "вне списка") {
		t.Fatalf("redirect out of the list: %d %q", exit, msg)
	}
	if exit, _, _ := runJob(t, j("/ok", `,"auth":{"Header":"X-Key","Secret":"NOPE"}`), nil); exit != 4 {
		t.Fatalf("missing secret: %d", exit)
	}
	if exit, _, _ := runJob(t, `{"url":"ftp://x"}`, nil); exit != 2 {
		t.Fatal("scheme")
	}
	if exit, _, _ := runJob(t, `nope`, nil); exit != 2 {
		t.Fatal("garbage")
	}
}

func TestAllowedHost(t *testing.T) {
	for host, want := range map[string]bool{"a.example.org": true, "example.org": false, "x.b.org": true, "B.org": false, "evil-example.org": false} {
		if got := allowedHost([]string{"a.example.org", "*.b.org"}, host); got != want {
			t.Errorf("%s: %v", host, got)
		}
	}
	if !allowedHost(nil, "anything") {
		t.Error("empty list is no limit")
	}
}
