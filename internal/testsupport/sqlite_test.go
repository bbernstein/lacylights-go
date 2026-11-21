package testsupport

import (
	"database/sql"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOpenMemoryDB_SingleConnection(t *testing.T) {
	db := OpenMemoryDB(t, sqlite.Open(":memory:"), &gorm.Config{})

	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Equal(t, 1, sqlDB.Stats().MaxOpenConnections)

	require.NoError(t, db.Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY)").Error)

	// Concurrent queries must all see the table: with a pool of more than one
	// connection a second connection would open a fresh, empty :memory: DB.
	const workers = 16
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var n int64
			errs <- db.Table("widgets").Count(&n).Error
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		assert.NoError(t, err)
	}
}

func TestOpenMemoryDB_ClosesOnCleanup(t *testing.T) {
	var db *gorm.DB
	t.Run("inner", func(t *testing.T) {
		db = OpenMemoryDB(t, sqlite.Open(":memory:"), &gorm.Config{})
	})
	sqlDB, err := db.DB()
	require.NoError(t, err)
	assert.Error(t, sqlDB.Ping(), "DB should be closed after the test's cleanup ran")
}

type fakeTB struct {
	testing.TB
	fatal string
}

func (f *fakeTB) Helper()                           {}
func (f *fakeTB) Cleanup(func())                    {}
func (f *fakeTB) Fatalf(format string, args ...any) { f.fatal = format; panic("fatal") }

func TestOpenMemoryDB_FatalOnError(t *testing.T) {
	ftb := &fakeTB{}
	// An invalid DSN option makes the sqlite driver fail on open.
	assert.Panics(t, func() {
		OpenMemoryDB(ftb, sqlite.Open("file:/nonexistent-dir/x.db?mode=ro"), &gorm.Config{})
	})
	assert.NotEmpty(t, ftb.fatal)
}

// notSQLDB wraps *sql.DB so gorm accepts it as a ConnPool, but db.DB() cannot
// unwrap it back to a *sql.DB.
type notSQLDB struct{ *sql.DB }

type wrappingDialector struct{ gorm.Dialector }

func (d wrappingDialector) Initialize(db *gorm.DB) error {
	if err := d.Dialector.Initialize(db); err != nil {
		return err
	}
	db.ConnPool = notSQLDB{db.ConnPool.(*sql.DB)}
	return nil
}

func TestOpenMemoryDB_FatalWhenSQLDBUnavailable(t *testing.T) {
	ftb := &fakeTB{}
	assert.Panics(t, func() {
		OpenMemoryDB(ftb, wrappingDialector{sqlite.Open(":memory:")}, &gorm.Config{})
	})
	assert.Contains(t, ftb.fatal, "sql.DB")
}

func TestOpenMemoryDB_NilConfig(t *testing.T) {
	db := OpenMemoryDB(t, sqlite.Open(":memory:"), nil)
	require.NotNil(t, db)
}
