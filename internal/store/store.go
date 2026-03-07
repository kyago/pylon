// Package store provides SQLite-based persistence for pylon.
// Spec Reference: Section 8 "SQLite Schema"
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
var migrationSQL string

// Store wraps a SQLite database connection for pylon data.
type Store struct {
	db *sql.DB
}

// NewStore opens a SQLite database at the given path and enables WAL mode.
// Use ":memory:" for in-memory testing.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &Store{db: db}, nil
}

// Migrate runs the embedded SQL migration to create all tables.
func (s *Store) Migrate() error {
	if _, err := s.db.Exec(migrationSQL); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced queries.
func (s *Store) DB() *sql.DB {
	return s.db
}
