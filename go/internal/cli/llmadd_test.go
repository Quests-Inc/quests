package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// fakeAssistant serves the two endpoints llm-add talks to and records the apply body.
func fakeAssistant(t *testing.T, preview string, applied *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/llm/actions/preview":
			_, _ = io.WriteString(w, preview)
		case "/api/llm/actions/apply":
			b, _ := io.ReadAll(r.Body)
			*applied = string(b)
			_, _ = io.WriteString(w, `{"ok":true,"results":[{"index":0,"action":"create_quest","is_new":true,"after":{"title":"Купить молоко"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return string(b)
}

func TestLLMAddAppliesPreviewedBatch(t *testing.T) {
	var applied string
	srv := fakeAssistant(t, `{"ok":true,"needs_clarification":false,"batch":{"actions":[{"type":"create_quest"}]},
		"preview":[{"index":0,"action":"create_quest","is_new":true,"after":{"title":"Купить молоко"}}]}`, &applied)
	defer srv.Close()

	var code int
	var err error
	out := captureStdout(t, func() {
		code, err = cmdLLMAdd(NewClient(srv.URL, false), []string{"-y", "купить", "молоко"})
	})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(out, "готово: ") || !strings.Contains(out, "Купить молоко") {
		t.Fatalf("unexpected output: %q", out)
	}
	var body struct {
		Batch json.RawMessage `json:"batch"`
	}
	if err := json.Unmarshal([]byte(applied), &body); err != nil || !strings.Contains(string(body.Batch), "create_quest") {
		t.Fatalf("batch not forwarded to apply: %q (%v)", applied, err)
	}
}

func TestLLMAddClarificationAppliesNothing(t *testing.T) {
	var applied string
	srv := fakeAssistant(t, `{"ok":false,"needs_clarification":true,"clarify_question":"Какой срок?"}`, &applied)
	defer srv.Close()

	code, err := cmdLLMAdd(NewClient(srv.URL, false), []string{"-y", "что-то"})
	if err != nil || code != 2 {
		t.Fatalf("want code 2, got %d err=%v", code, err)
	}
	if applied != "" {
		t.Fatalf("apply must not be called on clarification, got %q", applied)
	}
}

func TestLLMAddRequiresText(t *testing.T) {
	if _, err := cmdLLMAdd(NewClient("http://127.0.0.1:1", false), nil); err == nil {
		t.Fatal("want error on empty text")
	}
}
