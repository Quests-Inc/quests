package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/valirum/quests/go/internal/store"
)

// passOf orders the creating ops: containers first, then what lives in them.
func passOf(op store.DraftOp) int {
	switch op.Kind {
	case "create_quest", "create_note", "create_questline":
		return 1
	case "create_step":
		return 2
	case "upload_file":
		return 3
	case "copy":
		var p copyPayload
		if json.Unmarshal(op.Payload, &p) == nil {
			if src, ok := parseDraftRef(p.Source); ok {
				switch src.Type {
				case "step":
					return 2
				case "attachment":
					return 3
				}
				return 1
			}
		}
	}
	return 0
}

// postAttachment uploads bytes to an owner through the normal attachment
// endpoint (so scanning, change log and events are the usual ones).
func postAttachment(ctx context.Context, mux http.Handler, collection string, ownerID int64, filename, ctype, comment string, data []byte) (int64, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, strings.ReplaceAll(filename, `"`, "")))
	if ctype != "" {
		h.Set("Content-Type", ctype)
	}
	part, _ := mw.CreatePart(h)
	_, _ = part.Write(data)
	if comment != "" {
		_ = mw.WriteField("comment", comment)
	}
	_ = mw.Close()
	code, out := dispatch(ctx, mux, "POST", fmt.Sprintf("/api/%s/%d/attachments", collection, ownerID), mw.FormDataContentType(), buf.Bytes())
	return idFrom(code, out)
}

func getJSON(ctx context.Context, mux http.Handler, path string) (map[string]any, error) {
	code, out := dispatch(ctx, mux, "GET", path, "", nil)
	if code >= 300 {
		return nil, errors.New(errText(code, out))
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// pick copies the listed keys that are present.
func pick(src map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := src[k]; ok && v != nil {
			out[k] = v
		}
	}
	return out
}

const copySuffix = " (копия)"

// copyEntity makes the real copy a "copy" op stands for and returns its id.
// ownerRef is where a copied step or file goes (the original's owner unless
// the draft attaches it elsewhere).
func (s *Server) copyEntity(ctx context.Context, mux http.Handler, op store.DraftOp, ownerRef string,
	real func(string) (string, int64, bool), collection map[string]string) (int64, error) {
	var p copyPayload
	_ = json.Unmarshal(op.Payload, &p)
	src, ok := parseDraftRef(p.Source)
	if !ok {
		return 0, errors.New("bad copy source")
	}
	sid, _ := strconv.ParseInt(src.ID, 10, 64)
	create := func(path string, body map[string]any) (int64, error) {
		b, _ := json.Marshal(body)
		code, out := dispatch(ctx, mux, "POST", path, "application/json", b)
		return idFrom(code, out)
	}
	switch src.Type {
	case "quest":
		q, err := getJSON(ctx, mux, fmt.Sprintf("/api/quests/%d", sid))
		if err != nil {
			return 0, err
		}
		body := pick(q, "description", "significance", "category_id", "questline_id")
		body["title"] = fmt.Sprint(q["title"]) + copySuffix
		if tags, ok := q["tags"].([]any); ok {
			var ids []any
			for _, t := range tags {
				if m, ok := t.(map[string]any); ok && m["id"] != nil {
					ids = append(ids, m["id"])
				}
			}
			body["tag_ids"] = ids
		}
		if steps, ok := q["steps"].([]any); ok {
			var cp []any
			for _, st := range steps {
				if m, ok := st.(map[string]any); ok {
					c := pick(m, "title", "description", "progress_total", "sort_order")
					if t, _ := c["progress_total"].(float64); t < 1 {
						c["progress_total"] = 1
					}
					cp = append(cp, c)
				}
			}
			body["steps"] = cp
		}
		return create("/api/quests", body)
	case "note":
		n, err := getJSON(ctx, mux, fmt.Sprintf("/api/notes/%d", sid))
		if err != nil {
			return 0, err
		}
		body := pick(n, "description", "color", "icon", "parent_id")
		body["title"] = fmt.Sprint(n["title"]) + copySuffix
		return create("/api/notes", body)
	case "questline":
		l, err := getJSON(ctx, mux, fmt.Sprintf("/api/questlines/%d", sid))
		if err != nil {
			return 0, err
		}
		body := pick(l, "description", "color", "icon", "category_id")
		body["title"] = fmt.Sprint(l["title"]) + copySuffix
		return create("/api/questlines", body)
	case "step":
		_, qid, ok := real(ownerRef)
		if !ok {
			return 0, errors.New("copied step has no quest")
		}
		fromQ, err := s.Store.QuestIDForStep(ctx, sid)
		if err != nil {
			return 0, err
		}
		q, err := s.Store.GetQuest(ctx, fromQ)
		if err != nil {
			return 0, err
		}
		for _, st := range q.Steps {
			if st.ID != sid {
				continue
			}
			total := st.ProgressTotal
			if total < 1 {
				total = 1
			}
			b, _ := json.Marshal(map[string]any{
				"title": st.Title + copySuffix, "description": st.Description, "progress_total": total,
			})
			code, out := dispatch(ctx, mux, "POST", fmt.Sprintf("/api/quests/%d/steps", qid), "application/json", b)
			if code >= 300 {
				return 0, errors.New(errText(code, out))
			}
			return lastStepID(out)
		}
		return 0, store.ErrNotFound
	case "attachment":
		ot, oid, ok := real(ownerRef)
		if !ok {
			return 0, errors.New("copied file has no owner")
		}
		a, err := s.Store.Knowledge().GetAttachment(ctx, sid)
		if err != nil {
			return 0, err
		}
		rc, err := s.WebDAV.Get(ctx, a.WebDAVPath)
		if err != nil {
			return 0, fmt.Errorf("file unavailable: %w", err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			return 0, err
		}
		ctype := a.ContentTypeDeclared
		if ctype == "" {
			ctype = a.ContentTypeDetected
		}
		return postAttachment(ctx, mux, collection[ot], oid, a.Filename, ctype, a.Comment, buf.Bytes())
	}
	return 0, fmt.Errorf("cannot copy a %s", src.Type)
}

// putGraphDraftOp replaces the payload of a "create" op, so a thing that only
// exists in the draft can be edited in place.
func (s *Server) putGraphDraftOp(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return
	}
	var in struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	ops, err := s.Store.ListDraftOps(r.Context())
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	for _, o := range ops {
		if o.ID != id {
			continue
		}
		if o.Kind != "create_quest" && o.Kind != "create_note" && o.Kind != "create_step" && o.Kind != "create_questline" {
			writeErr(w, 422, "only created things can be edited in place")
			return
		}
		if msg := s.validateDraftOp(r.Context(), o.Kind, in.Payload, ops); msg != "" {
			writeErr(w, 422, msg)
			return
		}
		if err := s.Store.UpdateDraftOpPayload(r.Context(), id, in.Payload); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		o.Payload = in.Payload
		writeJSON(w, 200, o)
		return
	}
	writeErr(w, 404, "op not found")
}
