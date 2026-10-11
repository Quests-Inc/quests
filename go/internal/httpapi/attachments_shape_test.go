package httpapi

import (
	"encoding/json"
	"slices"
	"sort"
	"testing"
	"time"

	"github.com/quests-inc/quests/go/internal/knowledge"
	knowledgehttp "github.com/quests-inc/quests/go/internal/knowledge/http"
)

// The web client reads these keys by name: the shape of an attachment on the wire is part of the API.
func TestAttachmentJSONShape(t *testing.T) {
	a := knowledge.Attachment{ID: 1, OwnerType: "note", OwnerID: 2, Filename: "f.txt", UploadedAt: time.Unix(0, 0)}
	keys := func(v any) []string {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	base := []string{"comment", "content_type_declared", "content_type_detected", "filename", "hidden", "id", "owner_id",
		"owner_type", "revision", "revision_count", "scan_status", "scanned_at", "size_bytes", "uploaded_at"}
	live := slices.Concat(base, []string{"available", "last_modified", "source_updated"})
	sort.Strings(live)

	if got := keys(knowledgehttp.AttachmentView{AttachmentRead: knowledge.AttachmentToRead(a)}); !slices.Equal(got, base) {
		t.Errorf("unprobed keys = %v\nwant %v", got, base)
	}
	probed := knowledgehttp.AttachmentView{AttachmentRead: knowledge.AttachmentToRead(a), AttachmentLive: &knowledgehttp.AttachmentLive{}}
	if got := keys(probed); !slices.Equal(got, live) {
		t.Errorf("probed keys = %v\nwant %v", got, live)
	}
	rev := []string{"comment", "content_type_declared", "content_type_detected", "filename", "is_current", "revision",
		"scan_status", "scanned_at", "size_bytes", "uploaded_at"}
	if got := keys(knowledge.RevisionToRead(knowledge.AttachmentRevision{Revision: 1}, 1)); !slices.Equal(got, rev) {
		t.Errorf("revision keys = %v\nwant %v", got, rev)
	}
}
