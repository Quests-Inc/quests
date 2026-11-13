package wsarchive

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/quests-inc/quests/go/internal/platform/wsctx"
	"github.com/quests-inc/quests/go/internal/repository"
)

func (h *Handler) exportRepositories(ctx context.Context, zw *zip.Writer) error {
	lock := repository.WorkspaceLock(h.DataDir, wsctx.From(ctx))
	lock.RLock()
	defer lock.RUnlock()
	rows, err := h.Store.DB.QueryContext(ctx, `SELECT id FROM repository WHERE kind='local' ORDER BY id`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	_ = rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		dir := repository.Directory(h.DataDir, wsctx.From(ctx), id)
		// Empty repos are represented by their metadata; bundle cannot encode an empty ref set.
		refs, err := repository.Git(ctx, dir, "for-each-ref", "--format=%(refname)")
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			continue
		}
		temp, err := os.MkdirTemp("", "quests-repo-export-")
		if err != nil {
			return err
		}
		err = func() error {
			defer os.RemoveAll(temp) //nolint:errcheck
			bundle := filepath.Join(temp, "repo.bundle")
			if err := repository.Bundle(ctx, dir, bundle); err != nil {
				return err
			}
			f, err := os.Open(bundle)
			if err != nil {
				return err
			}
			defer f.Close()
			out, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("repositories/%d.bundle", id), Method: zip.Store})
			if err != nil {
				return err
			}
			_, err = io.Copy(out, f)
			return err
		}()
		if err != nil {
			return err
		}
	}
	return nil
}
func (h *Handler) importRepositories(ctx context.Context, entries map[string]*zip.File, ids map[string]map[int64]int64) error {
	var total int64
	for old, id := range ids["repository"] {
		var kind, branch string
		if err := h.Store.DB.QueryRowContext(ctx, `SELECT kind,branch FROM repository WHERE id=?`, id).Scan(&kind, &branch); err != nil {
			return err
		}
		if kind != "local" {
			continue
		}
		dir := repository.Directory(h.DataDir, wsctx.From(ctx), id)
		file := entries["repositories/"+strconv.FormatInt(old, 10)+".bundle"]
		if file == nil {
			if err := repository.Init(ctx, dir); err != nil {
				return err
			}
		} else {
			if file.UncompressedSize64 > uint64(repository.DefaultQuota) {
				return fmt.Errorf("репозиторий превышает квоту")
			}
			temp, err := os.MkdirTemp("", "quests-repo-import-")
			if err != nil {
				return err
			}
			err = func() error {
				defer os.RemoveAll(temp) //nolint:errcheck
				reader, err := file.Open()
				if err != nil {
					return err
				}
				defer reader.Close()
				f, err := os.Create(filepath.Join(temp, "repo.bundle"))
				if err != nil {
					return err
				}
				_, err = io.Copy(f, io.LimitReader(reader, repository.DefaultQuota+1))
				cerr := f.Close()
				if err != nil {
					return err
				}
				if cerr != nil {
					return cerr
				}
				return repository.RestoreBundle(ctx, dir, f.Name())
			}()
			if err != nil {
				return err
			}
		}
		// The branch is validated as a single ref by Git; no imported config or hooks are installed.
		if _, err := repository.Git(ctx, dir, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
			return err
		}
		size, err := repository.Bytes(dir)
		if err != nil {
			return err
		}
		total += size
		if total > repository.DefaultQuota {
			return fmt.Errorf("репозитории архива превышают квоту")
		}
	}
	return nil
}
