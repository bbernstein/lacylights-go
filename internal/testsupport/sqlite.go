// Package testsupport holds helpers shared by tests across the module.
package testsupport

import (
	"testing"

	"gorm.io/gorm"
)

// OpenMemoryDB opens an in-memory SQLite database restricted to a single
// pooled connection and closes it when the test finishes.
//
// Every connection to ":memory:" gets its own private, empty database, so a
// second pooled connection (for example one taken by a background goroutine)
// would see no tables. Limiting the pool to one connection makes all users
// share the same database. The dialector is supplied by the caller because
// tests use both the glebarez and gorm.io SQLite drivers.
func OpenMemoryDB(t testing.TB, dialector gorm.Dialector, cfg *gorm.Config) *gorm.DB {
	t.Helper()
	if cfg == nil {
		cfg = &gorm.Config{}
	}
	db, err := gorm.Open(dialector, cfg)
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("failed to get sql.DB: %v", err)
		return nil
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}
