package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// InitDB connects to the SQLite database and runs migrations.
// It assumes the DB file is located in the .wake directory of the project root.
func InitDB(projectRoot string) (*sql.DB, error) {
	wakeDir := filepath.Join(projectRoot, ".wake")
	if err := os.MkdirAll(wakeDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create .wake directory: %w", err)
	}

	// Create a .gitignore file in the .wake directory to ignore the database
	gitignorePath := filepath.Join(wakeDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		_ = os.WriteFile(gitignorePath, []byte("*\n"), 0600)
	}

	dsn := filepath.Join(wakeDir, "state.db") + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SEC-07: Single connection pool configuration prevents SQLITE_BUSY locking in WAL mode
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}
