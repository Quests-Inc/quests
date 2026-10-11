package knowledge

import (
	"context"
	"database/sql"
)

// AttachmentUsage is the bytes the context's workspace keeps on the file server. Every
// version of every attachment, the current one included, has a row in attachment_revision.
func (s *Store) AttachmentUsage(ctx context.Context) (int64, error) {
	var n sql.NullInt64
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(size_bytes), 0) FROM attachment_revision`).Scan(&n); err != nil {
		return 0, err
	}
	return n.Int64, nil
}
