package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/wsctx"
)

// Scripts and hooks go to the trash over HTTP: a hook comes back only after its script, a script erased for good takes its
// trashed hook along, and the hourly purge erases what is old enough.
func TestScriptAndHookTrashOverHTTP(t *testing.T) {
	h, s := secretsServer(t)
	call := func(method, path, body string) (int, any) {
		var in any
		if body != "" {
			in = json.RawMessage(body)
		}
		code, out := secretReq(h, method, path, in)
		return code, out
	}
	list := func(seg string) []map[string]any {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/trash/"+seg, nil))
		var out []map[string]any
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &out) != nil {
			t.Fatalf("trash %s: %d %s", seg, w.Code, w.Body.String())
		}
		return out
	}
	code, sc := secretReq(h, http.MethodPost, "/api/scripts", json.RawMessage(`{"title":"хук-скрипт","graph":`+hookScriptBody+`}`))
	if code != http.StatusCreated {
		t.Fatalf("script: %d %v", code, sc)
	}
	sid := jsonNum(sc["id"])
	code, hk := secretReq(h, http.MethodPost, "/api/hooks", json.RawMessage(`{"name":"пинг","events":["complete"],"script_id":`+sid+`,"run_on":"home"}`))
	if code != http.StatusCreated {
		t.Fatalf("hook: %d %v", code, hk)
	}
	hid := jsonNum(hk["id"])

	if code, _ := call(http.MethodDelete, "/api/scripts/"+sid, ""); code != http.StatusConflict {
		t.Fatalf("a script with a live hook stays: %d", code)
	}
	if code, _ := call(http.MethodDelete, "/api/hooks/"+hid, ""); code != http.StatusNoContent {
		t.Fatalf("hook to the trash: %d", code)
	}
	if code, _ := call(http.MethodDelete, "/api/scripts/"+sid, ""); code != http.StatusNoContent {
		t.Fatalf("script to the trash: %d", code)
	}
	hooks := list("hooks")
	if len(hooks) != 1 || hooks[0]["script_trashed"] != true || hooks[0]["title"] != "пинг" || hooks[0]["purge_at"] == "" {
		t.Fatalf("hook in the trash: %v", hooks)
	}
	if code, _ := call(http.MethodPost, "/api/trash/hooks/"+hid+"/restore", ""); code != http.StatusConflict {
		t.Fatalf("a hook before its script: %d", code)
	}
	if code, _ := call(http.MethodPost, "/api/trash/scripts/"+sid+"/restore", ""); code != http.StatusOK {
		t.Fatalf("restore the script: %d", code)
	}
	if code, _ := call(http.MethodPost, "/api/trash/hooks/"+hid+"/restore", ""); code != http.StatusOK {
		t.Fatalf("then the hook: %d", code)
	}
	if code, _ := call(http.MethodPost, "/api/trash/hooks/"+hid+"/restore", ""); code != http.StatusNotFound {
		t.Fatalf("a live hook is not in the trash: %d", code)
	}
	if code, _ := call(http.MethodPost, "/api/trash/hooks/x/restore", ""); code != http.StatusBadRequest {
		t.Fatalf("a bad id: %d", code)
	}

	// Erase one, empty the rest.
	_, _ = call(http.MethodDelete, "/api/hooks/"+hid, "")
	_, _ = call(http.MethodDelete, "/api/scripts/"+sid, "")
	if code, _ := call(http.MethodDelete, "/api/trash/scripts/"+sid, ""); code != http.StatusNoContent {
		t.Fatalf("erase the script: %d", code)
	}
	if code, _ := call(http.MethodDelete, "/api/trash/scripts/"+sid, ""); code != http.StatusNotFound {
		t.Fatalf("erased twice: %d", code)
	}
	if len(list("hooks")) != 0 || len(list("scripts")) != 0 {
		t.Fatal("the hook went with its script")
	}
	if code, _ := call(http.MethodDelete, "/api/trash/hooks", ""); code != http.StatusNoContent {
		t.Fatalf("empty: %d", code)
	}

	// The hourly purge: an old script goes, a fresh one stays.
	ctx := wsctx.With(t.Context(), 1)
	old, err := s.Store.Execution.CreateScript(ctx, "старый", "", []byte(hookScriptBody))
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := s.Store.Execution.CreateScript(ctx, "свежий", "", []byte(hookScriptBody))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{old.ID, fresh.ID} {
		if err := s.Store.Execution.DeleteScript(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	backdate(t, s, "script", old.ID, trashRetention+time.Hour)
	s.PurgeExpiredTrash(t.Context())
	if left := list("scripts"); len(left) != 1 || left[0]["title"] != "свежий" {
		t.Fatalf("only the fresh script is left: %v", left)
	}
}
