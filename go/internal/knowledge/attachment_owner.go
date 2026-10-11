package knowledge

import "context"

// SetAttachmentOwner re-parents an attachment. The WebDAV path is untouched:
// bytes are found through the stored path, not through the owner.
func (s *Store) SetAttachmentOwner(ctx context.Context, id int64, ownerType string, ownerID int64) (Attachment, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE attachment SET owner_type = ?, owner_id = ? WHERE id = ?`, ownerType, ownerID, id)
	if err != nil {
		return Attachment{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Attachment{}, ErrNotFound
	}
	return s.GetAttachment(ctx, id)
}
