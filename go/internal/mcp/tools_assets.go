package mcp

import (
	"bytes"
	"context"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// owner names whose attachments/icons a call is about; exactly one must be set.
type owner struct {
	Quest     *int64 `json:"quest,omitempty"`
	Questline *int64 `json:"questline,omitempty"`
	Note      *int64 `json:"note,omitempty"`
}

func (o owner) resolve(allowQuest bool) (kind, seg string, id int64, err error) {
	type kv struct {
		kind, seg string
		id        *int64
	}
	var set []kv
	for _, k := range []kv{{"quest", "quests", o.Quest}, {"questline", "questlines", o.Questline}, {"note", "notes", o.Note}} {
		if k.id != nil && (allowQuest || k.kind != "quest") {
			set = append(set, k)
		}
	}
	names := "quest, questline, note"
	if !allowQuest {
		names = "questline, note"
	}
	if len(set) != 1 || (!allowQuest && o.Quest != nil) {
		return "", "", 0, &Err{422, "provide exactly one of: " + names}
	}
	return set[0].kind, set[0].seg, *set[0].id, nil
}

const attachmentCap = 2 * 1024 * 1024 // above this a file is not even fetched into memory by the tool

func assetTools(c *Client) []Tool {
	type getArgs struct {
		owner
		ID       int64 `json:"id" desc:"attachment id"`
		Revision *int  `json:"revision,omitempty"`
		MaxChars int   `json:"max_chars,omitempty" desc:"text files: cut, default 4000"`
		Offset   int   `json:"offset,omitempty"`
	}
	get := Typed("get_attachment", "A file attached to a quest/questline/note: metadata and, for text files, the content (cut at max_chars, continue with offset). Binary files are saved to a temp file and its path is returned (saved_to). Needs the owner (quest/questline/note).",
		func(ctx context.Context, a getArgs) (any, error) {
			kind, seg, oid, err := a.resolve(true)
			if err != nil {
				return nil, err
			}
			var rows []map[string]any
			var meta map[string]any
			if err := c.Do(ctx, http.MethodGet, fmt.Sprintf("/api/%s/%d/attachments", seg, oid), values("stat", "0"), nil, &rows); err == nil {
				for _, r := range rows {
					if int64(num(r["id"])) == a.ID {
						meta = r
					}
				}
			}
			q := url.Values{}
			if a.Revision != nil {
				q.Set("revision", fmt.Sprint(*a.Revision))
			}
			if size := num(meta["size_bytes"]); size > attachmentCap {
				return map[string]any{"id": a.ID, "owner_type": kind, "owner_id": oid, "filename": meta["filename"], "size_bytes": size,
					"note": fmt.Sprintf("file is larger than %d bytes; not fetched", attachmentCap)}, nil
			}
			raw, _, err := c.Raw(ctx, fmt.Sprintf("/api/%s/%d/attachments/%d", seg, oid, a.ID), q)
			if err != nil {
				return nil, err
			}
			ct := str(meta["content_type_detected"])
			if ct == "" {
				ct = str(meta["content_type_declared"])
			}
			out := map[string]any{
				"id": a.ID, "owner_type": kind, "owner_id": oid, "filename": meta["filename"], "size_bytes": len(raw),
				"content_type": ct, "comment": meta["comment"], "hidden": meta["hidden"],
				"revision": meta["revision"], "revision_count": meta["revision_count"],
			}
			if looksText(ct, raw) {
				withText(out, "text", string(raw), a.MaxChars, a.Offset)
				return out, nil
			}
			dir := filepath.Join(os.TempDir(), "quests-attachments")
			name := filepath.Base(str(meta["filename"]))
			if name == "" || name == "." {
				name = "file"
			}
			path := filepath.Join(dir, fmt.Sprintf("%d-%s", a.ID, name))
			if err := os.MkdirAll(dir, 0o700); err == nil {
				err = os.WriteFile(path, raw, 0o600)
				if err == nil {
					out["saved_to"] = path
					return out, nil
				}
			}
			out["note"] = "binary file; could not be saved to a temp file"
			return out, nil
		})

	type hideArgs struct {
		owner
		ID     int64 `json:"id" desc:"attachment id"`
		Hidden bool  `json:"hidden"`
	}
	hide := Typed("set_attachment_hidden", "Hide or restore an attachment card (soft delete: gone from lists, still on the web Attachments tab).",
		func(ctx context.Context, a hideArgs) (any, error) {
			_, seg, oid, err := a.resolve(true)
			if err != nil {
				return nil, err
			}
			var row map[string]any
			if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/%s/%d/attachments/%d", seg, oid, a.ID), nil, map[string]any{"hidden": a.Hidden}, &row); err != nil {
				return nil, err
			}
			return ok(ctx, c, a.ID, map[string]any{"filename": row["filename"], "hidden": row["hidden"]}), nil
		})

	type upArgs struct {
		owner
		FilePath    string  `json:"file_path" desc:"local file"`
		Comment     *string `json:"comment,omitempty"`
		AsVersionOf *int64  `json:"as_version_of,omitempty" desc:"attachment id: upload a new revision of that card"`
	}
	upload := Typed("upload_attachment", "Attach a local file to a quest/questline/note (or as a new revision of an existing card). For note images: upload, then ![alt](attachment=N) in the text.",
		func(ctx context.Context, a upArgs) (any, error) {
			_, seg, oid, err := a.resolve(true)
			if err != nil {
				return nil, err
			}
			path := fmt.Sprintf("/api/%s/%d/attachments", seg, oid)
			if a.AsVersionOf != nil {
				path = fmt.Sprintf("%s/%d/revisions", path, *a.AsVersionOf)
			}
			extra := map[string]string{}
			if a.Comment != nil && *a.Comment != "" {
				extra["comment"] = *a.Comment
			}
			var row map[string]any
			if err := uploadFile(ctx, c, path, a.FilePath, extra, &row); err != nil {
				return nil, err
			}
			return ok(ctx, c, int64(num(row["id"])), map[string]any{"filename": row["filename"], "scan_status": row["scan_status"]}), nil
		})

	type iconArgs struct {
		Action    string  `json:"action" enum:"set,clear,color"`
		Questline *int64  `json:"questline,omitempty"`
		Note      *int64  `json:"note,omitempty"`
		FilePath  string  `json:"file_path,omitempty" desc:"set: local image"`
		Color     *string `json:"color,omitempty" desc:"color: hex like #c47a20"`
	}
	icon := Typed("icon", "A questline's or note's icon: action=set (file_path), clear, or color (hex).",
		func(ctx context.Context, a iconArgs) (any, error) {
			_, seg, oid, err := owner{Questline: a.Questline, Note: a.Note}.resolve(false)
			if err != nil {
				return nil, err
			}
			switch a.Action {
			case "set":
				if a.FilePath == "" {
					return nil, &Err{422, "set needs file_path"}
				}
				if err := uploadFile(ctx, c, fmt.Sprintf("/api/%s/%d/icon", seg, oid), a.FilePath, nil, nil); err != nil {
					return nil, err
				}
			case "clear":
				if err := c.Do(ctx, http.MethodDelete, fmt.Sprintf("/api/%s/%d/icon", seg, oid), nil, nil, nil); err != nil {
					return nil, err
				}
			case "color":
				hex := ""
				if a.Color != nil {
					hex = strings.TrimSpace(*a.Color)
				}
				if hex == "" {
					return nil, &Err{422, "color needs a hex value like #c47a20"}
				}
				if !strings.HasPrefix(hex, "#") {
					hex = "#" + hex
				}
				if err := c.Do(ctx, http.MethodPatch, fmt.Sprintf("/api/%s/%d", seg, oid), nil, map[string]any{"color": hex}, nil); err != nil {
					return nil, err
				}
			default:
				return nil, &Err{422, fmt.Sprintf("unknown action %q", a.Action)}
			}
			return ok(ctx, c, oid, map[string]any{"action": a.Action}), nil
		})

	return []Tool{get, hide, upload, icon}
}

// uploadFile posts a local file as multipart/form-data (field "file", plus extra text fields).
func uploadFile(ctx context.Context, c *Client, path, filePath string, extra map[string]string, out any) error {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return &Err{422, fmt.Sprintf("cannot read %s: %v", filePath, err)}
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range extra {
		_ = w.WriteField(k, v)
	}
	name := filepath.Base(filePath)
	ct := mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	h.Set("Content-Type", ct)
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	_, _ = part.Write(raw)
	_ = w.Close()
	return c.Upload(ctx, http.MethodPost, path, w.FormDataContentType(), &buf, out)
}

var textTypes = []string{"text/", "application/json", "application/xml", "application/javascript", "application/x-javascript",
	"application/yaml", "application/x-yaml", "application/toml", "+json", "+xml"}

func looksText(contentType string, raw []byte) bool {
	ct := strings.ToLower(contentType)
	for _, t := range textTypes {
		if strings.Contains(ct, t) {
			return true
		}
	}
	sample := raw
	if len(sample) > 512 {
		sample = sample[:512]
	}
	return !bytes.Contains(sample, []byte{0}) && utf8.Valid(sample)
}
