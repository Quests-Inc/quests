package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/quests-inc/quests/go/internal/platform/secretbox"
	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRepositorySecretIsEncryptedAndNotListed(t *testing.T) {
	s := newAttachmentServer(t, "", "", 0)
	box, err := secretbox.New(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	s.Secrets = box
	h := s.Handler()
	r := httptest.NewRequest("POST", "/api/repositories", strings.NewReader(`{"title":"Private","kind":"github","url":"https://github.com/team/project","token":"read-token-only"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	var result struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	var sealed string
	err = s.Store.DB.QueryRowContext(wsctx.With(context.Background(), 1), `SELECT value FROM repository_token WHERE repository_id=?`, result.ID).Scan(&sealed)
	if err != nil {
		t.Fatal(err)
	}
	if sealed == "read-token-only" {
		t.Fatal("plaintext stored")
	}
	plain, err := box.Open(sealed)
	if err != nil || plain != "read-token-only" {
		t.Fatal("token not recoverable")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/repositories", nil))
	if strings.Contains(w.Body.String(), "read-token-only") || strings.Contains(w.Body.String(), sealed) {
		t.Fatal("token leaked into list")
	}
	if !strings.Contains(w.Body.String(), `"has_token":true`) {
		t.Fatal("missing token indicator")
	}
}
