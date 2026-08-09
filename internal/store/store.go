// Package store defines the database abstraction layer.
//
// The application depends on the Store interface (not on *gorm.DB directly)
// so a Postgres (or other) backend can be dropped in later without touching
// handlers. See AGENTS.md ("Database abstraction layer").
//
// Today the only implementation is NewSQLite, backed by gorm + the pure-Go
// SQLite driver (modernc.org/sqlite). Handlers reach the *gorm.DB via DB();
// future work should move queries into repository methods on this interface
// so the SQL dialect becomes the backend's responsibility.
package store

import "gorm.io/gorm"

// Store is the database abstraction the app depends on.
type Store interface {
	// DB returns the underlying *gorm.DB for repository-style queries.
	DB() *gorm.DB
	// Close releases the connection pool.
	Close() error
}

type sqlStore struct{ db *gorm.DB }

func (s *sqlStore) DB() *gorm.DB { return s.db }

func (s *sqlStore) Close() error {
	if s.db == nil {
		return nil
	}
	if sqlDB, err := s.db.DB(); err == nil {
		return sqlDB.Close()
	}
	return nil
}

// NewSQLite wraps an already-opened *gorm.DB as a Store.
func NewSQLite(db *gorm.DB) Store { return &sqlStore{db: db} }
