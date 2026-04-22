package db

import (
	"fmt"
	"time"

	"github.com/timae/ses/internal/model"
)

type LinkedSession struct {
	model.Session
	Reason    string
	Direction string // "to" or "from"
	LinkedAt  time.Time
}

func (db *DB) AddLink(fromShortID, toShortID, reason string) error {
	from, err := db.GetSession(fromShortID)
	if err != nil {
		return fmt.Errorf("source session: %w", err)
	}
	to, err := db.GetSession(toShortID)
	if err != nil {
		return fmt.Errorf("target session: %w", err)
	}

	_, err = db.conn.Exec(
		`INSERT OR IGNORE INTO session_links (from_id, to_id, reason) VALUES (?, ?, ?)`,
		from.ID, to.ID, reason)
	return err
}

func (db *DB) RemoveLink(fromShortID, toShortID string) error {
	from, err := db.GetSession(fromShortID)
	if err != nil {
		return err
	}
	to, err := db.GetSession(toShortID)
	if err != nil {
		return err
	}

	db.conn.Exec("DELETE FROM session_links WHERE from_id = ? AND to_id = ?", from.ID, to.ID)
	db.conn.Exec("DELETE FROM session_links WHERE from_id = ? AND to_id = ?", to.ID, from.ID)
	return nil
}

func (db *DB) GetLinkedSessions(shortID string) ([]LinkedSession, error) {
	s, err := db.GetSession(shortID)
	if err != nil {
		return nil, err
	}

	rows, err := db.conn.Query(`
		SELECT s.id, s.short_id, s.source_type, s.source_id, s.project,
			s.started_at, s.first_prompt, s.model, s.transcript_path,
			l.reason, l.created_at, 'to'
		FROM session_links l
		JOIN sessions s ON s.id = l.to_id
		WHERE l.from_id = ?
		UNION ALL
		SELECT s.id, s.short_id, s.source_type, s.source_id, s.project,
			s.started_at, s.first_prompt, s.model, s.transcript_path,
			l.reason, l.created_at, 'from'
		FROM session_links l
		JOIN sessions s ON s.id = l.from_id
		WHERE l.to_id = ?
		ORDER BY started_at`, s.ID, s.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var linked []LinkedSession
	for rows.Next() {
		var ls LinkedSession
		err := rows.Scan(
			&ls.ID, &ls.ShortID, &ls.SourceType, &ls.SourceID, &ls.Project,
			&ls.StartedAt, &ls.FirstPrompt, &ls.Model, &ls.TranscriptPath,
			&ls.Reason, &ls.LinkedAt, &ls.Direction)
		if err != nil {
			continue
		}
		linked = append(linked, ls)
	}
	return linked, nil
}
