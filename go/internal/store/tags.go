package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/valirum/quests/go/internal/journal"
	"github.com/valirum/quests/go/internal/platform/timeutil"
)

const MaxTagsPerOwner = 5

var (
	ErrTooManyTags = fmt.Errorf("at most %d tags", MaxTagsPerOwner)
	ErrBadSlug     = fmt.Errorf("invalid tag slug")
)

var nonSlug = regexp.MustCompile(`[^a-z0-9а-яё-]+`)

// NormalizeTagSlug lowercases, trims, turns whitespace/underscores into '-',
// strips other punctuation. Empty after normalize → invalid.
func NormalizeTagSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, "ё", "е")
	s = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '_' {
			return '-'
		}
		return r
	}, s)
	s = nonSlug.ReplaceAllString(s, "")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

var tagPalette = []string{
	"#5a8a9a", "#8a8578", "#7a9e3a", "#6a7ab8", "#c47a20",
	"#b54a3a", "#9a6a9a", "#3a9a7a", "#c9a227", "#6e8a9a",
}

func RandomTagColor() string {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return tagPalette[0]
	}
	return tagPalette[int(b[0])%len(tagPalette)]
}

func (s *Store) ListTags(ctx context.Context, q string) ([]journal.Tag, error) {
	sqlq := `SELECT id, slug, label, color FROM tag`
	args := []any{}
	q = strings.TrimSpace(q)
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		sqlq += ` WHERE lower(slug) LIKE ? OR lower(label) LIKE ?`
		args = append(args, like, like)
	}
	sqlq += ` ORDER BY slug`
	rows, err := s.DB.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]journal.Tag, 0)
	for rows.Next() {
		var t journal.Tag
		if err := rows.Scan(&t.ID, &t.Slug, &t.Label, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTag(ctx context.Context, id int64) (journal.Tag, error) {
	var t journal.Tag
	err := s.DB.QueryRowContext(ctx, `SELECT id, slug, label, color FROM tag WHERE id = ?`, id).
		Scan(&t.ID, &t.Slug, &t.Label, &t.Color)
	if err == sql.ErrNoRows {
		return journal.Tag{}, ErrNotFound
	}
	return t, err
}

func (s *Store) GetTagBySlug(ctx context.Context, slug string) (journal.Tag, error) {
	slug = NormalizeTagSlug(slug)
	var t journal.Tag
	err := s.DB.QueryRowContext(ctx, `SELECT id, slug, label, color FROM tag WHERE slug = ?`, slug).
		Scan(&t.ID, &t.Slug, &t.Label, &t.Color)
	if err == sql.ErrNoRows {
		return journal.Tag{}, ErrNotFound
	}
	return t, err
}

type TagCreate struct {
	Slug  string
	Label string
	Color string
}

func (s *Store) CreateTag(ctx context.Context, in TagCreate) (journal.Tag, error) {
	slug := NormalizeTagSlug(in.Slug)
	if slug == "" {
		slug = NormalizeTagSlug(in.Label)
	}
	if slug == "" {
		return journal.Tag{}, ErrBadSlug
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = slug
	}
	color := strings.TrimSpace(in.Color)
	if color == "" {
		color = RandomTagColor()
	}
	if !strings.HasPrefix(color, "#") {
		color = "#" + color
	}
	now := timeutil.NowUTC()
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO tag (slug, label, color, created_at) VALUES (?, ?, ?, ?)`,
		slug, label, color, timeutil.ToDBUTC(now))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			existing, gerr := s.GetTagBySlug(ctx, slug)
			if gerr == nil {
				return existing, ErrConflict
			}
			return journal.Tag{}, ErrConflict
		}
		return journal.Tag{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return journal.Tag{}, err
	}
	return journal.Tag{ID: id, Slug: slug, Label: label, Color: color}, nil
}

func (s *Store) UpdateTag(ctx context.Context, id int64, label, color *string) (journal.Tag, error) {
	t, err := s.GetTag(ctx, id)
	if err != nil {
		return journal.Tag{}, err
	}
	if label != nil {
		l := strings.TrimSpace(*label)
		if l != "" {
			t.Label = l
		}
	}
	if color != nil {
		c := strings.TrimSpace(*color)
		if c != "" {
			if !strings.HasPrefix(c, "#") {
				c = "#" + c
			}
			t.Color = c
		}
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE tag SET label = ?, color = ? WHERE id = ?`, t.Label, t.Color, id)
	if err != nil {
		return journal.Tag{}, err
	}
	return t, nil
}

func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM tag WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) loadTagsForQuests(ctx context.Context, questIDs []int64) (map[int64][]journal.Tag, error) {
	out := map[int64][]journal.Tag{}
	if len(questIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(questIDs))
	args := make([]any, len(questIDs))
	for i, id := range questIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
		SELECT qt.quest_id, t.id, t.slug, t.label, t.color
		FROM quest_tag qt
		JOIN tag t ON t.id = qt.tag_id
		WHERE qt.quest_id IN (%s)
		ORDER BY t.slug`, strings.Join(placeholders, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var qid int64
		var t journal.Tag
		if err := rows.Scan(&qid, &t.ID, &t.Slug, &t.Label, &t.Color); err != nil {
			return nil, err
		}
		out[qid] = append(out[qid], t)
	}
	return out, rows.Err()
}

func (s *Store) loadTagsForTemplates(ctx context.Context, templateIDs []int64) (map[int64][]journal.Tag, error) {
	out := map[int64][]journal.Tag{}
	if len(templateIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(templateIDs))
	args := make([]any, len(templateIDs))
	for i, id := range templateIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`
		SELECT tt.template_id, t.id, t.slug, t.label, t.color
		FROM template_tag tt
		JOIN tag t ON t.id = tt.tag_id
		WHERE tt.template_id IN (%s)
		ORDER BY t.slug`, strings.Join(placeholders, ","))
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var tid int64
		var t journal.Tag
		if err := rows.Scan(&tid, &t.ID, &t.Slug, &t.Label, &t.Color); err != nil {
			return nil, err
		}
		out[tid] = append(out[tid], t)
	}
	return out, rows.Err()
}

func (s *Store) attachTagsToQuests(ctx context.Context, quests []journal.Quest) error {
	ids := make([]int64, len(quests))
	for i := range quests {
		ids[i] = quests[i].ID
	}
	m, err := s.loadTagsForQuests(ctx, ids)
	if err != nil {
		return err
	}
	for i := range quests {
		tags := m[quests[i].ID]
		if tags == nil {
			tags = []journal.Tag{}
		}
		quests[i].Tags = tags
	}
	return nil
}

func (s *Store) SetQuestTags(ctx context.Context, questID int64, tagIDs []int64) error {
	if len(tagIDs) > MaxTagsPerOwner {
		return ErrTooManyTags
	}
	seen := map[int64]struct{}{}
	uniq := make([]int64, 0, len(tagIDs))
	for _, id := range tagIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	for _, id := range uniq {
		if _, err := s.GetTag(ctx, id); err != nil {
			return err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM quest_tag WHERE quest_id = ?`, questID); err != nil {
		return err
	}
	for _, tid := range uniq {
		if _, err := tx.ExecContext(ctx, `INSERT INTO quest_tag (quest_id, tag_id) VALUES (?, ?)`, questID, tid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetTemplateTags(ctx context.Context, templateID int64, tagIDs []int64) error {
	if len(tagIDs) > MaxTagsPerOwner {
		return ErrTooManyTags
	}
	seen := map[int64]struct{}{}
	uniq := make([]int64, 0, len(tagIDs))
	for _, id := range tagIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		uniq = append(uniq, id)
	}
	for _, id := range uniq {
		if _, err := s.GetTag(ctx, id); err != nil {
			return err
		}
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM template_tag WHERE template_id = ?`, templateID); err != nil {
		return err
	}
	for _, tid := range uniq {
		if _, err := tx.ExecContext(ctx, `INSERT INTO template_tag (template_id, tag_id) VALUES (?, ?)`, templateID, tid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CopyTemplateTagsToQuest(ctx context.Context, templateID, questID int64) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT tag_id FROM template_tag WHERE template_id = ?`, templateID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > MaxTagsPerOwner {
		ids = ids[:MaxTagsPerOwner]
	}
	return s.SetQuestTags(ctx, questID, ids)
}

// ResolveTagIDs maps a mixed list of numeric ids and slugs to tag ids.
func (s *Store) ResolveTagIDs(ctx context.Context, refs []string) ([]int64, error) {
	out := make([]int64, 0, len(refs))
	seen := map[int64]struct{}{}
	for _, raw := range refs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var id int64
		if _, err := fmt.Sscan(raw, &id); err == nil && fmt.Sprintf("%d", id) == raw {
			if _, err := s.GetTag(ctx, id); err != nil {
				return nil, err
			}
		} else {
			t, err := s.GetTagBySlug(ctx, raw)
			if err != nil {
				// try label match
				t, err = s.findTagByLabel(ctx, raw)
				if err != nil {
					return nil, err
				}
			}
			id = t.ID
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > MaxTagsPerOwner {
		return nil, ErrTooManyTags
	}
	return out, nil
}

func (s *Store) findTagByLabel(ctx context.Context, label string) (journal.Tag, error) {
	label = strings.TrimSpace(label)
	var t journal.Tag
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, slug, label, color FROM tag WHERE lower(label) = lower(?) LIMIT 1`, label).
		Scan(&t.ID, &t.Slug, &t.Label, &t.Color)
	if err == sql.ErrNoRows {
		return journal.Tag{}, ErrNotFound
	}
	return t, err
}
