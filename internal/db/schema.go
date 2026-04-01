package db

const schemaSQL = `
CREATE TABLE IF NOT EXISTS sessions (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    short_id         TEXT UNIQUE NOT NULL,
    source_type      TEXT NOT NULL,
    source_id        TEXT NOT NULL,
    pid              INTEGER DEFAULT 0,
    project          TEXT NOT NULL,
    cwd              TEXT NOT NULL,
    git_branch       TEXT DEFAULT '',
    git_commit       TEXT DEFAULT '',
    started_at       DATETIME NOT NULL,
    ended_at         DATETIME,
    message_count    INTEGER DEFAULT 0,
    tool_call_count  INTEGER DEFAULT 0,
    first_prompt     TEXT DEFAULT '',
    last_assistant   TEXT DEFAULT '',
    model            TEXT DEFAULT '',
    transcript_path  TEXT NOT NULL,
    scanned_at       DATETIME NOT NULL,
    transcript_mtime DATETIME,
    transcript_size  INTEGER,
    UNIQUE(source_type, source_id)
);

CREATE TABLE IF NOT EXISTS session_tags (
    session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    tag        TEXT NOT NULL,
    PRIMARY KEY (session_id, tag)
);

CREATE TABLE IF NOT EXISTS session_files (
    session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    file_path  TEXT NOT NULL,
    action     TEXT DEFAULT 'unknown',
    PRIMARY KEY (session_id, file_path)
);

CREATE TABLE IF NOT EXISTS user_prompts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    ordinal    INTEGER NOT NULL,
    content    TEXT NOT NULL,
    timestamp  DATETIME
);

CREATE VIRTUAL TABLE IF NOT EXISTS session_fts USING fts5(
    short_id,
    project,
    first_prompt,
    last_assistant,
    prompts_text,
    content=sessions,
    content_rowid=id
);

CREATE TABLE IF NOT EXISTS session_links (
    from_id    INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    to_id      INTEGER REFERENCES sessions(id) ON DELETE CASCADE,
    reason     TEXT DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (from_id, to_id)
);

CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at DESC);
CREATE INDEX IF NOT EXISTS idx_sessions_project ON sessions(project);
CREATE INDEX IF NOT EXISTS idx_sessions_source  ON sessions(source_type);
CREATE INDEX IF NOT EXISTS idx_session_links_from ON session_links(from_id);
CREATE INDEX IF NOT EXISTS idx_session_links_to   ON session_links(to_id);
`
