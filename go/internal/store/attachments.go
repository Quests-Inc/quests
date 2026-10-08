package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/valirum/quests/go/internal/timeutil"
)

// MaxAttachmentRevisions caps history per card. Uploading beyond this drops
// the oldest non-current revision (and its WebDAV object).
const MaxAttachmentRevisions = 20

// Attachment is the logical file card. File fields mirror the *current*
// revision so listings stay a single-row read; history lives in
// attachment_revision. attachment=N refs point at ID.
type Attachment struct {
	ID                  int64
	OwnerType           string
	OwnerID             int64
	Filename            string
	WebDAVPath          string
	SizeBytes           int64
	ContentTypeDeclared string
	ContentTypeDetected string
	Comment             string
	UploadedAt          time.Time
	ScanStatus          string
	ScannedAt           *time.Time
	CurrentRevision     int
	Hidden              bool
	RevisionCount       int // filled by list/get helpers; not a DB column
}

// AttachmentRevision is one immutable version of the file on WebDAV.
type AttachmentRevision struct {
	ID                  int64
	AttachmentID        int64
	Revision            int
	Filename            string
	WebDAVPath          string
	SizeBytes           int64
	ContentTypeDeclared string
	ContentTypeDetected string
	Comment             string
	UploadedAt          time.Time
	ScanStatus          string
	ScannedAt           *time.Time
}

type AttachmentRead map[string]any

const attachmentCols = `id, owner_type, owner_id, filename, webdav_path, size_bytes,
	content_type_declared, content_type_detected, comment, uploaded_at, scan_status, scanned_at,
	current_revision, hidden`

const revisionCols = `id, attachment_id, revision, filename, webdav_path, size_bytes,
	content_type_declared, content_type_detected, comment, uploaded_at, scan_status, scanned_at`

func scanAttachment(row rowScanner) (Attachment, error) {
	var a Attachment
	var uploaded string
	var scanned sql.NullString
	err := row.Scan(&a.ID, &a.OwnerType, &a.OwnerID, &a.Filename, &a.WebDAVPath, &a.SizeBytes,
		&a.ContentTypeDeclared, &a.ContentTypeDetected, &a.Comment, &uploaded, &a.ScanStatus, &scanned,
		&a.CurrentRevision, &a.Hidden)
	if err != nil {
		return a, err
	}
	if a.CurrentRevision < 1 {
		a.CurrentRevision = 1
	}
	if t, err := timeutil.ParseFlexible(uploaded); err == nil {
		a.UploadedAt = t
	}
	if scanned.Valid && scanned.String != "" {
		if t, err := timeutil.ParseFlexible(scanned.String); err == nil {
			a.ScannedAt = &t
		}
	}
	return a, nil
}

func scanRevision(row rowScanner) (AttachmentRevision, error) {
	var r AttachmentRevision
	var uploaded string
	var scanned sql.NullString
	err := row.Scan(&r.ID, &r.AttachmentID, &r.Revision, &r.Filename, &r.WebDAVPath, &r.SizeBytes,
		&r.ContentTypeDeclared, &r.ContentTypeDetected, &r.Comment, &uploaded, &r.ScanStatus, &scanned)
	if err != nil {
		return r, err
	}
	if t, err := timeutil.ParseFlexible(uploaded); err == nil {
		r.UploadedAt = t
	}
	if scanned.Valid && scanned.String != "" {
		if t, err := timeutil.ParseFlexible(scanned.String); err == nil {
			r.ScannedAt = &t
		}
	}
	return r, nil
}

// AttachmentToRead shapes the JSON. WebDAVPath is deliberately absent.
func AttachmentToRead(a Attachment) AttachmentRead {
	rev := a.CurrentRevision
	if rev < 1 {
		rev = 1
	}
	count := a.RevisionCount
	if count < 1 {
		count = 1
	}
	return AttachmentRead{
		"id":                    a.ID,
		"owner_type":            a.OwnerType,
		"owner_id":              a.OwnerID,
		"filename":              a.Filename,
		"size_bytes":            a.SizeBytes,
		"content_type_declared": a.ContentTypeDeclared,
		"content_type_detected": a.ContentTypeDetected,
		"comment":               a.Comment,
		"uploaded_at":           timeutil.ToUTCISO(&a.UploadedAt),
		"scan_status":           a.ScanStatus,
		"scanned_at":            timeutil.ToUTCISO(a.ScannedAt),
		"revision":              rev,
		"revision_count":        count,
		"hidden":                a.Hidden,
	}
}

func RevisionToRead(r AttachmentRevision, current int) AttachmentRead {
	return AttachmentRead{
		"revision":              r.Revision,
		"filename":              r.Filename,
		"size_bytes":            r.SizeBytes,
		"content_type_declared": r.ContentTypeDeclared,
		"content_type_detected": r.ContentTypeDetected,
		"comment":               r.Comment,
		"uploaded_at":           timeutil.ToUTCISO(&r.UploadedAt),
		"scan_status":           r.ScanStatus,
		"scanned_at":            timeutil.ToUTCISO(r.ScannedAt),
		"is_current":            r.Revision == current,
	}
}

func (s *Store) fillRevisionCounts(ctx context.Context, rows []Attachment) error {
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		n, err := s.CountAttachmentRevisions(ctx, rows[i].ID)
		if err != nil {
			return err
		}
		rows[i].RevisionCount = n
	}
	return nil
}

func (s *Store) ListAttachments(ctx context.Context, ownerType string, ownerID int64) ([]Attachment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+attachmentCols+`
		FROM attachment WHERE owner_type = ? AND owner_id = ?
		ORDER BY uploaded_at, id`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Attachment, 0)
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillRevisionCounts(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListAllAttachments(ctx context.Context) ([]Attachment, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+attachmentCols+`
		FROM attachment
		ORDER BY owner_type, owner_id, uploaded_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Attachment, 0)
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.fillRevisionCounts(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetAttachment(ctx context.Context, id int64) (Attachment, error) {
	row := s.DB.QueryRowContext(ctx, `SELECT `+attachmentCols+` FROM attachment WHERE id = ?`, id)
	a, err := scanAttachment(row)
	if err == sql.ErrNoRows {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, err
	}
	n, err := s.CountAttachmentRevisions(ctx, a.ID)
	if err != nil {
		return Attachment{}, err
	}
	a.RevisionCount = n
	return a, nil
}

func (s *Store) CountAttachmentRevisions(ctx context.Context, attachmentID int64) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM attachment_revision WHERE attachment_id = ?`, attachmentID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if n < 1 {
		return 1, nil
	}
	return n, nil
}

func (s *Store) ListAttachmentRevisions(ctx context.Context, attachmentID int64) ([]AttachmentRevision, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT `+revisionCols+`
		FROM attachment_revision WHERE attachment_id = ?
		ORDER BY revision`, attachmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AttachmentRevision, 0)
	for rows.Next() {
		r, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetAttachmentRevision(ctx context.Context, attachmentID int64, revision int) (AttachmentRevision, error) {
	row := s.DB.QueryRowContext(ctx, `
		SELECT `+revisionCols+`
		FROM attachment_revision WHERE attachment_id = ? AND revision = ?`, attachmentID, revision)
	r, err := scanRevision(row)
	if err == sql.ErrNoRows {
		return AttachmentRevision{}, ErrNotFound
	}
	return r, err
}

func (s *Store) CreateAttachment(ctx context.Context, a Attachment) (Attachment, error) {
	now := timeutil.NowUTC()
	if a.UploadedAt.IsZero() {
		a.UploadedAt = now
	}
	if a.CurrentRevision < 1 {
		a.CurrentRevision = 1
	}
	var scanned any
	if a.ScannedAt != nil {
		scanned = timeutil.ToDBUTC(*a.ScannedAt)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Attachment{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO attachment (owner_type, owner_id, filename, webdav_path, size_bytes,
			content_type_declared, content_type_detected, comment, uploaded_at, scan_status, scanned_at,
			current_revision, hidden)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.OwnerType, a.OwnerID, a.Filename, a.WebDAVPath, a.SizeBytes,
		a.ContentTypeDeclared, a.ContentTypeDetected, a.Comment,
		timeutil.ToDBUTC(a.UploadedAt), a.ScanStatus, scanned, a.CurrentRevision, boolInt(a.Hidden))
	if err != nil {
		return Attachment{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Attachment{}, err
	}
	a.ID = id
	_, err = tx.ExecContext(ctx, `
		INSERT INTO attachment_revision (attachment_id, revision, filename, webdav_path, size_bytes,
			content_type_declared, content_type_detected, comment, uploaded_at, scan_status, scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.CurrentRevision, a.Filename, a.WebDAVPath, a.SizeBytes,
		a.ContentTypeDeclared, a.ContentTypeDetected, a.Comment,
		timeutil.ToDBUTC(a.UploadedAt), a.ScanStatus, scanned)
	if err != nil {
		return Attachment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attachment{}, err
	}
	a.RevisionCount = 1
	return a, nil
}

// AddAttachmentRevision stores a new version, makes it current, and returns
// the updated card plus any WebDAV paths that should be deleted (rotation).
func (s *Store) AddAttachmentRevision(ctx context.Context, attachmentID int64, rev AttachmentRevision) (Attachment, []string, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Attachment{}, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var maxRev int
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(revision), 0) FROM attachment_revision WHERE attachment_id = ?`,
		attachmentID).Scan(&maxRev)
	if err != nil {
		return Attachment{}, nil, err
	}
	next := maxRev + 1
	rev.Revision = next
	rev.AttachmentID = attachmentID
	if rev.UploadedAt.IsZero() {
		rev.UploadedAt = timeutil.NowUTC()
	}
	var scanned any
	if rev.ScannedAt != nil {
		scanned = timeutil.ToDBUTC(*rev.ScannedAt)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO attachment_revision (attachment_id, revision, filename, webdav_path, size_bytes,
			content_type_declared, content_type_detected, comment, uploaded_at, scan_status, scanned_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		attachmentID, next, rev.Filename, rev.WebDAVPath, rev.SizeBytes,
		rev.ContentTypeDeclared, rev.ContentTypeDetected, rev.Comment,
		timeutil.ToDBUTC(rev.UploadedAt), rev.ScanStatus, scanned)
	if err != nil {
		return Attachment{}, nil, err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE attachment SET filename=?, webdav_path=?, size_bytes=?,
			content_type_declared=?, content_type_detected=?, comment=?,
			uploaded_at=?, scan_status=?, scanned_at=?, current_revision=?
		WHERE id=?`,
		rev.Filename, rev.WebDAVPath, rev.SizeBytes,
		rev.ContentTypeDeclared, rev.ContentTypeDetected, rev.Comment,
		timeutil.ToDBUTC(rev.UploadedAt), rev.ScanStatus, scanned, next, attachmentID)
	if err != nil {
		return Attachment{}, nil, err
	}

	var dropPaths []string
	var count int
	err = tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM attachment_revision WHERE attachment_id = ?`, attachmentID).Scan(&count)
	if err != nil {
		return Attachment{}, nil, err
	}
	for count > MaxAttachmentRevisions {
		var oldID int64
		var oldPath string
		var oldRev int
		err = tx.QueryRowContext(ctx, `
			SELECT id, webdav_path, revision FROM attachment_revision
			WHERE attachment_id = ? AND revision != ?
			ORDER BY revision ASC LIMIT 1`, attachmentID, next).Scan(&oldID, &oldPath, &oldRev)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return Attachment{}, nil, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM attachment_revision WHERE id = ?`, oldID); err != nil {
			return Attachment{}, nil, err
		}
		dropPaths = append(dropPaths, oldPath)
		count--
	}

	if err := tx.Commit(); err != nil {
		return Attachment{}, nil, err
	}
	a, err := s.GetAttachment(ctx, attachmentID)
	return a, dropPaths, err
}

// SetCurrentAttachmentRevision copies an existing revision onto the card.
func (s *Store) SetCurrentAttachmentRevision(ctx context.Context, attachmentID int64, revision int) (Attachment, error) {
	rev, err := s.GetAttachmentRevision(ctx, attachmentID, revision)
	if err != nil {
		return Attachment{}, err
	}
	var scanned any
	if rev.ScannedAt != nil {
		scanned = timeutil.ToDBUTC(*rev.ScannedAt)
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE attachment SET filename=?, webdav_path=?, size_bytes=?,
			content_type_declared=?, content_type_detected=?, comment=?,
			uploaded_at=?, scan_status=?, scanned_at=?, current_revision=?
		WHERE id=?`,
		rev.Filename, rev.WebDAVPath, rev.SizeBytes,
		rev.ContentTypeDeclared, rev.ContentTypeDetected, rev.Comment,
		timeutil.ToDBUTC(rev.UploadedAt), rev.ScanStatus, scanned, rev.Revision, attachmentID)
	if err != nil {
		return Attachment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Attachment{}, ErrNotFound
	}
	return s.GetAttachment(ctx, attachmentID)
}

func (s *Store) SetAttachmentHidden(ctx context.Context, id int64, hidden bool) (Attachment, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE attachment SET hidden = ? WHERE id = ?`, boolInt(hidden), id)
	if err != nil {
		return Attachment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Attachment{}, ErrNotFound
	}
	return s.GetAttachment(ctx, id)
}

func (s *Store) SetAttachmentComment(ctx context.Context, id int64, comment string) (Attachment, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return Attachment{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var curRev int
	err = tx.QueryRowContext(ctx, `SELECT current_revision FROM attachment WHERE id = ?`, id).Scan(&curRev)
	if err == sql.ErrNoRows {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE attachment SET comment = ? WHERE id = ?`, comment, id); err != nil {
		return Attachment{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE attachment_revision SET comment = ? WHERE attachment_id = ? AND revision = ?`,
		comment, id, curRev); err != nil {
		return Attachment{}, err
	}
	if err := tx.Commit(); err != nil {
		return Attachment{}, err
	}
	return s.GetAttachment(ctx, id)
}

// DeleteAttachmentRevision removes one non-current revision. Returns its WebDAV path.
func (s *Store) DeleteAttachmentRevision(ctx context.Context, attachmentID int64, revision int) (string, error) {
	a, err := s.GetAttachment(ctx, attachmentID)
	if err != nil {
		return "", err
	}
	if revision == a.CurrentRevision {
		return "", ErrConflict
	}
	rev, err := s.GetAttachmentRevision(ctx, attachmentID, revision)
	if err != nil {
		return "", err
	}
	res, err := s.DB.ExecContext(ctx,
		`DELETE FROM attachment_revision WHERE attachment_id = ? AND revision = ?`,
		attachmentID, revision)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return rev.WebDAVPath, nil
}

func (s *Store) DeleteAttachment(ctx context.Context, id int64) error {
	// CASCADE on attachment_revision when FK is present; also explicit for sqlite test schemas.
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM attachment_revision WHERE attachment_id = ?`, id)
	res, err := s.DB.ExecContext(ctx, `DELETE FROM attachment WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AttachmentPathsForOwner returns every WebDAV path (all revisions) for an owner.
func (s *Store) AttachmentPathsForOwner(ctx context.Context, ownerType string, ownerID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT r.webdav_path FROM attachment_revision r
		JOIN attachment a ON a.id = r.attachment_id
		WHERE a.owner_type = ? AND a.owner_id = ?`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	seen := map[string]bool{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Fallback for rows with no revision yet (shouldn't happen after migration).
	if len(out) == 0 {
		rows2, err := s.DB.QueryContext(ctx,
			`SELECT webdav_path FROM attachment WHERE owner_type = ? AND owner_id = ?`, ownerType, ownerID)
		if err != nil {
			return nil, err
		}
		defer rows2.Close()
		for rows2.Next() {
			var p string
			if err := rows2.Scan(&p); err != nil {
				return nil, err
			}
			out = append(out, p)
		}
		return out, rows2.Err()
	}
	return out, nil
}

func (s *Store) DeleteAttachmentsForOwner(ctx context.Context, ownerType string, ownerID int64) error {
	ids, err := s.DB.QueryContext(ctx,
		`SELECT id FROM attachment WHERE owner_type = ? AND owner_id = ?`, ownerType, ownerID)
	if err != nil {
		return err
	}
	defer ids.Close()
	var list []int64
	for ids.Next() {
		var id int64
		if err := ids.Scan(&id); err != nil {
			return err
		}
		list = append(list, id)
	}
	if err := ids.Err(); err != nil {
		return err
	}
	for _, id := range list {
		if _, err := s.DB.ExecContext(ctx, `DELETE FROM attachment_revision WHERE attachment_id = ?`, id); err != nil {
			return err
		}
	}
	_, err = s.DB.ExecContext(ctx,
		`DELETE FROM attachment WHERE owner_type = ? AND owner_id = ?`, ownerType, ownerID)
	return err
}
