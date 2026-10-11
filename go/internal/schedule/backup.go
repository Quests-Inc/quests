package schedule

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/quests-inc/quests/go/internal/platform/health"
	"github.com/quests-inc/quests/go/internal/platform/webdav"
	"github.com/quests-inc/quests/go/internal/store"
)

// dumpDatabase writes the snapshot of the database at url to outPath (pg_dump; tests replace it).
var dumpDatabase = pgDump

// backupSubdir is where local snapshots live, under the data dir — sibling
// to the DB itself, not the OS temp dir, so it survives on the same volume
// that gets backed up/restored as a unit.
const backupSubdir = "backups"

// webdavBackupDir is the directory on the WebDAV server backups upload to,
// separate from the "attachments" tree that already lives there.
const webdavBackupDir = "backups"

// RunBackupLoop periodically snapshots the DB (pg_dump), builds
// an attachments archive from that snapshot, uploads to WebDAV / remote PC when
// configured, and prunes old pairs. interval<=0 disables it entirely.
func RunBackupLoop(ctx context.Context, st *store.Store, wd *webdav.Client, dataDir string, interval time.Duration, keep int, remote BackupRemote, reg *health.Registry) {
	if interval <= 0 {
		log.Printf("backup: disabled (QUESTS_BACKUP_INTERVAL_HOURS=0)")
		return
	}
	rs := NewSFTPRemote(remote)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := RunBackupOnce(ctx, st, wd, dataDir, keep, rs, reg); err != nil {
			log.Printf("backup: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunBackupOnce flushes pending remote uploads, takes one snapshot+archive,
// uploads, and prunes down to `keep`. Failures at upload stages keep the local
// copy and retry on the next tick.
func RunBackupOnce(ctx context.Context, st *store.Store, wd *webdav.Client, dataDir string, keep int, remote RemoteStore, reg *health.Registry) error {
	dir := filepath.Join(dataDir, backupSubdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		reg.SetProbe("backup", "offline", err.Error())
		return fmt.Errorf("mkdir: %w", err)
	}

	if remote != nil && remote.Configured() {
		flushPendingRemote(ctx, st, dir, remote)
	}

	now := time.Now().UTC()
	const mime = "application/octet-stream"
	filename := fmt.Sprintf("quests-%s-%09d.dump", now.Format("20060102-150405"), now.Nanosecond())
	localPath := filepath.Join(dir, filename)

	if snapErr := dumpDatabase(ctx, st.DatabaseURL, localPath); snapErr != nil {
		reg.SetProbe("backup", "offline", snapErr.Error())
		_, _ = st.InsertBackupLog(ctx, store.BackupInsert{Filename: filename, Error: snapErr.Error()})
		return fmt.Errorf("snapshot: %w", snapErr)
	}

	info, err := os.Stat(localPath)
	if err != nil {
		reg.SetProbe("backup", "offline", err.Error())
		_, _ = st.InsertBackupLog(ctx, store.BackupInsert{Filename: filename, Error: err.Error()})
		return fmt.Errorf("stat snapshot: %w", err)
	}

	attName := ""
	var attSize int64
	if wd != nil && wd.Configured() {
		attName = attachmentsArchiveName(filename)
		attPath := filepath.Join(dir, attName)
		var sz int64
		paths, archErr := listAllWebDAVPaths(ctx, st)
		if archErr == nil {
			sz, archErr = buildAttachmentsArchive(ctx, wd, paths, attPath)
		}
		if archErr != nil {
			log.Printf("backup: attachments archive %s: %v", attName, archErr)
			_ = os.Remove(attPath)
			attName = ""
			attSize = 0
		} else {
			attSize = sz
		}
	}

	id, err := st.InsertBackupLog(ctx, store.BackupInsert{
		Filename:             filename,
		SizeBytes:            info.Size(),
		AttachmentsFilename:  attName,
		AttachmentsSizeBytes: attSize,
	})
	if err != nil {
		return fmt.Errorf("record backuplog: %w", err)
	}

	webdavOK := uploadDBToWebDAV(ctx, st, wd, id, localPath, filename, mime, info.Size())
	remoteOK := false
	if remote != nil && remote.Configured() {
		remoteOK = uploadPairToRemote(ctx, st, remote, id, dir, filename, attName)
	}

	detail := fmt.Sprintf("%s (%d bytes, webdav=%v, remote=%v, attachments=%d)",
		filename, info.Size(), webdavOK, remoteOK, attSize)
	reg.SetProbe("backup", "ok", detail)

	if err := pruneBackups(ctx, st, wd, remote, dir, keep); err != nil {
		log.Printf("backup: prune: %v", err)
	}
	return nil
}

func uploadDBToWebDAV(ctx context.Context, st *store.Store, wd *webdav.Client, id int64, localPath, filename, mime string, size int64) bool {
	if wd == nil || !wd.Configured() {
		return false
	}
	f, err := os.Open(localPath)
	if err != nil {
		log.Printf("backup: reopen %s for webdav: %v", filename, err)
		return false
	}
	putErr := wd.Put(ctx, webdavBackupDir+"/"+filename, mime, f, size)
	f.Close()
	if putErr != nil {
		log.Printf("backup: webdav upload %s: %v", filename, putErr)
		return false
	}
	if err := st.MarkBackupWebdavUploaded(ctx, id); err != nil {
		log.Printf("backup: mark webdav uploaded %s: %v", filename, err)
		return false
	}
	return true
}

func uploadPairToRemote(ctx context.Context, st *store.Store, remote RemoteStore, id int64, dir, dbName, attName string) bool {
	dbPath := filepath.Join(dir, dbName)
	if _, err := os.Stat(dbPath); err != nil {
		log.Printf("backup: remote missing local %s: %v", dbName, err)
		return false
	}
	if err := remote.Upload(ctx, dbPath, dbName); err != nil {
		log.Printf("backup: remote upload %s: %v", dbName, err)
		return false
	}
	if attName != "" {
		attPath := filepath.Join(dir, attName)
		if _, err := os.Stat(attPath); err == nil {
			if err := remote.Upload(ctx, attPath, attName); err != nil {
				log.Printf("backup: remote upload %s: %v", attName, err)
				return false
			}
		}
	}
	if err := st.MarkBackupRemoteUploaded(ctx, id); err != nil {
		log.Printf("backup: mark remote uploaded %s: %v", dbName, err)
		return false
	}
	return true
}

// flushPendingRemote retries SFTP for rows that never reached the PC (offline queue).
func flushPendingRemote(ctx context.Context, st *store.Store, dir string, remote RemoteStore) {
	rows, err := st.ListBackupLogs(ctx)
	if err != nil {
		log.Printf("backup: flush list: %v", err)
		return
	}
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		if r.RemoteUploaded || r.Error != "" || r.Filename == "" {
			continue
		}
		if uploadPairToRemote(ctx, st, remote, r.ID, dir, r.Filename, r.AttachmentsFilename) {
			log.Printf("backup: flushed pending remote %s", r.Filename)
		}
	}
}

// pruneBackups keeps the `keep` most recent rows and removes the rest: local
// db+archive, WebDAV db (if uploaded), remote pair (if uploaded), and the row.
func pruneBackups(ctx context.Context, st *store.Store, wd *webdav.Client, remote RemoteStore, dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	rows, err := st.ListBackupLogs(ctx)
	if err != nil {
		return err
	}
	if len(rows) <= keep {
		return nil
	}
	for _, r := range rows[keep:] {
		if err := os.Remove(filepath.Join(dir, r.Filename)); err != nil && !os.IsNotExist(err) {
			log.Printf("backup: prune local %s: %v", r.Filename, err)
		}
		if r.AttachmentsFilename != "" {
			if err := os.Remove(filepath.Join(dir, r.AttachmentsFilename)); err != nil && !os.IsNotExist(err) {
				log.Printf("backup: prune local %s: %v", r.AttachmentsFilename, err)
			}
		}
		if r.WebdavUploaded && wd != nil && wd.Configured() {
			if err := wd.Delete(ctx, webdavBackupDir+"/"+r.Filename); err != nil && err != webdav.ErrNotFound {
				log.Printf("backup: prune webdav %s: %v", r.Filename, err)
			}
		}
		if r.RemoteUploaded && remote != nil && remote.Configured() {
			if err := remote.Remove(ctx, r.Filename); err != nil {
				log.Printf("backup: prune remote %s: %v", r.Filename, err)
			}
			if r.AttachmentsFilename != "" {
				if err := remote.Remove(ctx, r.AttachmentsFilename); err != nil {
					log.Printf("backup: prune remote %s: %v", r.AttachmentsFilename, err)
				}
			}
		}
		if err := st.DeleteBackupLog(ctx, r.ID); err != nil {
			log.Printf("backup: prune backuplog row %d: %v", r.ID, err)
		}
	}
	return nil
}
