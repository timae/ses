package db

// ToolOutputRow is one stored tool_result — the content Claude's UI hides
// behind the Ctrl+O collapse. Kept in its own table because these rows are
// much larger than user prompts and we don't want to bloat the FTS/hot path.
type ToolOutputRow struct {
	Ordinal  int
	ToolName string
	FilePath string
	Size     int
	Content  string
}

// GetToolOutputs returns all tool outputs for a session in capture order.
func (db *DB) GetToolOutputs(sessionID int64) ([]ToolOutputRow, error) {
	rows, err := db.conn.Query(
		`SELECT ordinal, tool_name, file_path, size, content
		 FROM tool_outputs
		 WHERE session_id = ?
		 ORDER BY ordinal`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolOutputRow
	for rows.Next() {
		var r ToolOutputRow
		if err := rows.Scan(&r.Ordinal, &r.ToolName, &r.FilePath, &r.Size, &r.Content); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
