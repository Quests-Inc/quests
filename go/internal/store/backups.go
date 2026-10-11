package store

import (
	"context"

	"github.com/quests-inc/quests/go/internal/platform/timeutil"
)

// BackupRow is one row of backuplog — one attempted DB snapshot (+ optional
// attachments archive), with upload flags for WebDAV and the remote PC sink.
type BackupRow struct {
	ID                   int64
	Filename             string
	SizeBytes            int64
	CreatedAt            string // RFC3339 UTC, as stored
	WebdavUploaded       bool
	RemoteUploaded       bool
	AttachmentsFilename  string
	AttachmentsSizeBytes int64
	Error                string
}

// BackupInsert is the payload for a new backuplog row.
type BackupInsert struct {
	Filename             string
	SizeBytes            int64
	AttachmentsFilename  string
	AttachmentsSizeBytes int64
	WebdavUploaded       bool
	RemoteUploaded       bool
	Error                string
}

// InsertBackupLog records an attempted snapshot (successful or not — Error
// non-empty marks a failed attempt, kept for visibility rather than dropped).
func (s *Store) InsertBackupLog(ctx context.Context, in BackupInsert) (int64, error) {
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO backuplog (
			filename, size_bytes, created_at, webdav_uploaded, remote_uploaded,
			attachments_filename, attachments_size_bytes, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Filename, in.SizeBytes, timeutil.ToDBUTC(timeutil.NowUTC()),
		boolInt(in.WebdavUploaded), boolInt(in.RemoteUploaded),
		nullStrVal(in.AttachmentsFilename), nullInt64(in.AttachmentsSizeBytes),
		nullStrVal(in.Error),
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarkBackupWebdavUploaded flips webdav_uploaded=1 once the WebDAV PUT succeeds.
func (s *Store) MarkBackupWebdavUploaded(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE backuplog SET webdav_uploaded=1 WHERE id=?`, id)
	return err
}

// MarkBackupUploaded is kept as an alias for older call sites / clarity in
// WebDAV-only flows.
func (s *Store) MarkBackupUploaded(ctx context.Context, id int64) error {
	return s.MarkBackupWebdavUploaded(ctx, id)
}

// MarkBackupRemoteUploaded flips remote_uploaded=1 once both artifacts land on the PC.
func (s *Store) MarkBackupRemoteUploaded(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE backuplog SET remote_uploaded=1 WHERE id=?`, id)
	return err
}

// ListBackupLogs returns every row, newest first — the rotation source of
// truth (not a WebDAV / remote directory listing).
func (s *Store) ListBackupLogs(ctx context.Context) ([]BackupRow, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, filename, size_bytes, created_at,
			webdav_uploaded, remote_uploaded,
			COALESCE(attachments_filename, ''), COALESCE(attachments_size_bytes, 0),
			COALESCE(error, '')
		FROM backuplog ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BackupRow
	for rows.Next() {
		var r BackupRow
		var webdavUp, remoteUp int
		if err := rows.Scan(
			&r.ID, &r.Filename, &r.SizeBytes, &r.CreatedAt,
			&webdavUp, &remoteUp,
			&r.AttachmentsFilename, &r.AttachmentsSizeBytes,
			&r.Error,
		); err != nil {
			return nil, err
		}
		r.WebdavUploaded = webdavUp != 0
		r.RemoteUploaded = remoteUp != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBackupLog(ctx context.Context, id int64) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM backuplog WHERE id=?`, id)
	return err
}

func nullStrVal(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}
