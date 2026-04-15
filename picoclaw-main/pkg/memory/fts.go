package memory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// FTSEntry is a single searchable memory record.
type FTSEntry struct {
	ID         int64
	SessionKey string
	Role       string
	Content    string
	CreatedAt  time.Time
	Rank       float64 // Search relevance (lower = more relevant with FTS5)
}

// FTSStore provides full-text search over conversation memory using SQLite FTS5.
// It sits alongside the existing JSONLStore — it does NOT replace it.
// Use Index() to feed messages from JSONL into the FTS index, and Search()
// to find them by natural language query.
type FTSStore struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

// NewFTSStore opens or creates a SQLite database with an FTS5 virtual table.
// The dbPath should be inside the memory directory, e.g. "memory/fts.db".
func NewFTSStore(dbPath string) (*FTSStore, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("fts: open db: %w", err)
	}

	// Create tables
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS memory_entries (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			session_key TEXT    NOT NULL,
			role        TEXT    NOT NULL,
			content     TEXT    NOT NULL,
			created_at  TEXT    NOT NULL
		);
		CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
			content,
			session_key UNINDEXED,
			role        UNINDEXED,
			content=memory_entries,
			content_rowid=id,
			tokenize='unicode61 remove_diacritics 2'
		);
		CREATE TRIGGER IF NOT EXISTS memory_fts_ai AFTER INSERT ON memory_entries BEGIN
			INSERT INTO memory_fts(rowid, content, session_key, role)
			VALUES (new.id, new.content, new.session_key, new.role);
		END;
		CREATE TRIGGER IF NOT EXISTS memory_fts_ad AFTER DELETE ON memory_entries BEGIN
			INSERT INTO memory_fts(memory_fts, rowid, content, session_key, role)
			VALUES ('delete', old.id, old.content, old.session_key, old.role);
		END;
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("fts: create tables: %w", err)
	}

	return &FTSStore{db: db, path: dbPath}, nil
}

// Index adds a message to the FTS index. Idempotent — duplicate
// (session_key, content, created_at) tuples are silently skipped.
func (f *FTSStore) Index(ctx context.Context, sessionKey, role, content string, createdAt time.Time) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	_, err := f.db.ExecContext(ctx,
		`INSERT INTO memory_entries (session_key, role, content, created_at)
		 VALUES (?, ?, ?, ?)`,
		sessionKey, role, content, createdAt.UTC().Format(time.RFC3339))
	return err
}

// Search finds messages matching the query, ordered by relevance.
// Optional sessionKey filters to a single session ("" = all sessions).
func (f *FTSStore) Search(ctx context.Context, query string, sessionKey string, limit int) ([]FTSEntry, error) {
	if limit <= 0 {
		limit = 10
	}

	// Sanitize the FTS5 query: wrap each word in double quotes to prevent
	// syntax injection (FTS5 operators like AND/OR/NOT/NEAR).
	safeQuery := sanitizeFTSQuery(query)
	if safeQuery == "" {
		return nil, nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	var rows *sql.Rows
	var err error

	if sessionKey != "" {
		rows, err = f.db.QueryContext(ctx,
			`SELECT e.id, e.session_key, e.role, e.content, e.created_at, rank
			 FROM memory_fts f
			 JOIN memory_entries e ON e.id = f.rowid
			 WHERE memory_fts MATCH ?
			   AND f.session_key = ?
			 ORDER BY rank
			 LIMIT ?`,
			safeQuery, sessionKey, limit)
	} else {
		rows, err = f.db.QueryContext(ctx,
			`SELECT e.id, e.session_key, e.role, e.content, e.created_at, rank
			 FROM memory_fts f
			 JOIN memory_entries e ON e.id = f.rowid
			 WHERE memory_fts MATCH ?
			 ORDER BY rank
			 LIMIT ?`,
			safeQuery, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("fts: search: %w", err)
	}
	defer rows.Close()

	var entries []FTSEntry
	for rows.Next() {
		var e FTSEntry
		var ts string
		if err := rows.Scan(&e.ID, &e.SessionKey, &e.Role, &e.Content, &ts, &e.Rank); err != nil {
			return nil, fmt.Errorf("fts: scan row: %w", err)
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Count returns the total number of indexed entries.
func (f *FTSStore) Count(ctx context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM memory_entries").Scan(&count)
	return count, err
}

// Close releases the database connection.
func (f *FTSStore) Close() error {
	if f.db != nil {
		return f.db.Close()
	}
	return nil
}

// sanitizeFTSQuery wraps each whitespace-separated token in double quotes
// to prevent FTS5 syntax injection.
func sanitizeFTSQuery(q string) string {
	words := strings.Fields(q)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		// Strip existing quotes and wrap
		w = strings.ReplaceAll(w, `"`, ``)
		if w == "" {
			continue
		}
		b.WriteByte('"')
		b.WriteString(w)
		b.WriteByte('"')
	}
	return b.String()
}
