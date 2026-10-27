package httpapi

import (
	"context"
	"github.com/quests-inc/quests/go/internal/platform/pgtest"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"net/http"
	"testing"
)

func TestContainerTrashPreservesLinksAndDisablesRestoredTemplate(t *testing.T) {
	h, s := secretsServer(t)
	qid := seedQuest(t, s)
	pgtest.Seed(t, s.Store.DB, `UPDATE quest SET questline_id=7,template_id=5`)
	for _, base := range []string{"/api/templates/5", "/api/questlines/7"} {
		code, _ := secretReq(h, http.MethodDelete, base, nil)
		if code != 204 {
			t.Fatalf("delete %s: %d", base, code)
		}
		code, _ = secretReq(h, http.MethodGet, base, nil)
		if code != 404 {
			t.Fatalf("hidden %s: %d", base, code)
		}
	}
	var line, template int64
	if err := s.Store.DB.QueryRow(`SELECT questline_id,template_id FROM quest WHERE id=?`, qid).Scan(&line, &template); err != nil || line != 7 || template != 5 {
		t.Fatalf("links: %d %d %v", line, template, err)
	}
	for _, base := range []string{"/api/trash/questlines/7", "/api/trash/templates/5"} {
		if code, _ := secretReq(h, http.MethodPost, base+"/restore", nil); code != 204 {
			t.Fatalf("restore: %d", code)
		}
	}
	var enabled int
	_ = s.Store.DB.QueryRow(`SELECT enabled FROM questtemplate WHERE id=5`).Scan(&enabled)
	if enabled != 0 {
		t.Fatal("restored template must not emit until explicitly enabled")
	}
	for _, base := range []string{"/api/templates/5", "/api/questlines/7"} {
		if code, _ := secretReq(h, http.MethodDelete, base, nil); code != 204 {
			t.Fatal(code)
		}
	}
	for _, base := range []string{"/api/trash/templates/5", "/api/trash/questlines/7"} {
		if code, _ := secretReq(h, http.MethodDelete, base, nil); code != 204 {
			t.Fatalf("purge: %d", code)
		}
	}
	var detached bool
	_ = s.Store.DB.QueryRow(`SELECT questline_id IS NULL AND template_id IS NULL FROM quest WHERE id=?`, qid).Scan(&detached)
	if !detached {
		t.Fatal("purge must preserve independent quest")
	}
}
func TestStarterEndpointsAndContainerRetention(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	h := s.Handler()
	if code, _ := secretReq(h, http.MethodPost, "/api/workspace/starter", nil); code != 200 {
		t.Fatalf("seed: %d", code)
	}
	if code, out := secretReq(h, http.MethodPost, "/api/workspace/starter", nil); code != 200 || out["added"] != float64(0) {
		t.Fatalf("repeat: %d %v", code, out)
	}
	if code, _ := secretReq(h, http.MethodDelete, "/api/workspace/starter", nil); code != 200 {
		t.Fatalf("remove: %d", code)
	}
	for _, kind := range []string{"template", "questline"} {
		list, err := s.Store.ListTrashedContainers(wsctx.With(context.Background(), 1), kind)
		if err != nil || len(list) != 1 {
			t.Fatalf("%s: %+v %v", kind, list, err)
		}
	}
	tx, err := s.Store.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`SELECT set_config('app.trash','1',true)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE questtemplate SET deleted_at=now()-interval '31 days'`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`UPDATE questline SET deleted_at=now()-interval '31 days'`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s.PurgeExpiredTrash(context.Background())
	for _, kind := range []string{"template", "questline"} {
		list, err := s.Store.ListTrashedContainers(wsctx.With(context.Background(), 1), kind)
		if err != nil || len(list) != 0 {
			t.Fatalf("retention %s: %+v %v", kind, list, err)
		}
	}
}

func TestFirstPersonalWorkspaceGetsExamplesOnlyOnce(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	user, err := s.Store.Identity.CreateUser("starter-owner", "correcthorse")
	if err != nil {
		t.Fatal(err)
	}
	ws, err := EnsureUserWorkspace(context.Background(), s.Store, user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	ctx := wsctx.With(context.Background(), ws.ID)
	var count int
	if err := s.Store.DB.QueryRowContext(ctx, `SELECT count(*) FROM seed_item`).Scan(&count); err != nil || count == 0 {
		t.Fatalf("personal: %d %v", count, err)
	}
	next, err := ProvisionWorkspace(context.Background(), s.Store, user.ID, "empty next", ws.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB.QueryRowContext(wsctx.With(context.Background(), next.ID), `SELECT count(*) FROM seed_item`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("later workspace: %d %v", count, err)
	}
}
