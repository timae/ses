package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/timae/ses/internal/model"
)

// PasteRow is one row returned by ListPastes: a single user turn with enough
// session metadata to recognize it in a listing.
type PasteRow struct {
	SessionShortID string
	SessionProject string
	SessionSource  model.Source
	SessionStarted time.Time
	Ordinal        int
	Size           int
	Content        string
}

// PasteFilter mirrors ListFilter but adds a minimum-size threshold on the
// paste itself. Shape filtering is applied in the caller (CLI) because
// shape classification is a heuristic, not a column.
type PasteFilter struct {
	MinSize int
	Since   *time.Time
	Until   *time.Time
	Project string
	Source  string
	Limit   int // final row cap; 0 means unlimited
}

// ListPastes returns user turns whose content length >= MinSize, newest
// session first, paste size descending within a session. Caller does shape
// filtering and final truncation after reading.
func (db *DB) ListPastes(f PasteFilter) ([]PasteRow, error) {
	if f.MinSize < 0 {
		f.MinSize = 0
	}

	query := `SELECT s.short_id, s.project, s.source_type, s.started_at,
	                 p.ordinal, LENGTH(p.content), p.content
	          FROM user_prompts p
	          JOIN sessions s ON s.id = p.session_id`

	var conditions []string
	var args []any

	conditions = append(conditions, "LENGTH(p.content) >= ?")
	args = append(args, f.MinSize)

	if f.Since != nil {
		conditions = append(conditions, "s.started_at >= ?")
		args = append(args, *f.Since)
	}
	if f.Until != nil {
		conditions = append(conditions, "s.started_at <= ?")
		args = append(args, *f.Until)
	}
	if f.Project != "" {
		conditions = append(conditions, "s.project LIKE ?")
		args = append(args, "%"+f.Project+"%")
	}
	if f.Source != "" {
		conditions = append(conditions, "s.source_type = ?")
		args = append(args, f.Source)
	}

	query += " WHERE " + strings.Join(conditions, " AND ")
	query += " ORDER BY LENGTH(p.content) DESC"
	if f.Limit > 0 {
		// Fetch a multiple so a post-hoc shape filter still has material to
		// work with; the CLI truncates to the real limit after filtering.
		fetchLimit := f.Limit * 20
		if fetchLimit > 5000 {
			fetchLimit = 5000
		}
		query += fmt.Sprintf(" LIMIT %d", fetchLimit)
	}

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PasteRow
	for rows.Next() {
		var r PasteRow
		if err := rows.Scan(
			&r.SessionShortID, &r.SessionProject, &r.SessionSource, &r.SessionStarted,
			&r.Ordinal, &r.Size, &r.Content,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
