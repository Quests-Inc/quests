package schedule

import (
	"archive/tar"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"github.com/valirum/quests/go/internal/platform/webdav"
	"github.com/valirum/quests/go/internal/store"
)

// attachmentsArchiveName derives the sidecar archive name from the DB snapshot
// filename (quests-….dump → quests-…-attachments.tar.zst).
func attachmentsArchiveName(dbFilename string) string {
	base := strings.TrimSuffix(strings.TrimSuffix(dbFilename, ".db"), ".dump")
	if base == dbFilename || base == "" {
		base = dbFilename
	}
	return base + "-attachments.tar.zst"
}

// listAllWebDAVPaths collects attachment paths from every workspace of the live
// database (row-level security shows one workspace at a time).
func listAllWebDAVPaths(ctx context.Context, st *store.Store) ([]string, error) {
	var out []string
	var firstErr error
	seen := map[string]struct{}{}
	err := st.Identity().ForEachWorkspace(ctx, func(wctx context.Context, _ int64) {
		paths, err := listWebDAVPaths(wctx, st.DB)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return
		}
		for _, p := range paths {
			if _, ok := seen[p]; !ok {
				seen[p] = struct{}{}
				out = append(out, p)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return out, firstErr
}

// listWebDAVPaths returns every distinct webdav_path referenced by attachment
// metadata (current rows and revisions) in db.
func listWebDAVPaths(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT webdav_path FROM attachment WHERE webdav_path IS NOT NULL AND webdav_path != ''
		UNION
		SELECT webdav_path FROM attachment_revision WHERE webdav_path IS NOT NULL AND webdav_path != ''
		ORDER BY 1`)
	if err != nil {
		// attachment_revision may be missing on older schemas.
		rows, err = db.QueryContext(ctx, `
			SELECT webdav_path FROM attachment
			WHERE webdav_path IS NOT NULL AND webdav_path != ''
			ORDER BY 1`)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var out []string
	seen := map[string]struct{}{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, rows.Err()
}

// buildAttachmentsArchive writes a zstd-compressed tar of every WebDAV object
// listed in the DB snapshot. Missing objects (404) are skipped with a log line.
// Returns bytes written (compressed file size).
func buildAttachmentsArchive(ctx context.Context, wd *webdav.Client, paths []string, archivePath string) (int64, error) {

	f, err := os.Create(archivePath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	zw, err := zstd.NewWriter(f)
	if err != nil {
		return 0, err
	}
	tw := tar.NewWriter(zw)

	for _, davPath := range paths {
		if err := ctx.Err(); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return 0, err
		}
		body, err := wd.Get(ctx, davPath)
		if err != nil {
			if err == webdav.ErrNotFound {
				log.Printf("backup: attachments skip missing %s", davPath)
				continue
			}
			_ = tw.Close()
			_ = zw.Close()
			return 0, fmt.Errorf("get %s: %w", davPath, err)
		}
		data, readErr := io.ReadAll(body)
		body.Close()
		if readErr != nil {
			_ = tw.Close()
			_ = zw.Close()
			return 0, fmt.Errorf("read %s: %w", davPath, readErr)
		}
		name := path.Clean("/" + strings.TrimPrefix(davPath, "/"))
		if name == "/" || name == "." {
			name = path.Base(davPath)
		} else {
			name = strings.TrimPrefix(name, "/")
		}
		hdr := &tar.Header{
			Name:    name,
			Mode:    0o644,
			Size:    int64(len(data)),
			ModTime: time.Now().UTC(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return 0, err
		}
		if _, err := tw.Write(data); err != nil {
			_ = tw.Close()
			_ = zw.Close()
			return 0, err
		}
	}

	if err := tw.Close(); err != nil {
		_ = zw.Close()
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}
