// Package database provides database connection and management.
package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite" // Pure Go SQLite driver (no CGO required)
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the global database connection.
var DB *gorm.DB

// Config holds database configuration.
type Config struct {
	URL         string
	MaxIdleConn int
	MaxOpenConn int
	Debug       bool
}

// Connect establishes a connection to the database.
func Connect(cfg Config) (*gorm.DB, error) {
	// Parse the DATABASE_URL (format: "file:./path/to/db" or just path)
	dbPath := strings.TrimPrefix(cfg.URL, "file:")

	// Ensure the directory exists
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// Configure GORM logger
	var logLevel logger.LogLevel
	if cfg.Debug {
		logLevel = logger.Info
	} else {
		logLevel = logger.Silent
	}

	gormLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	// Open the database with SQLite-specific settings
	// Note: We use the same pragmas as Prisma for compatibility
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000", dbPath)

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:                 gormLogger,
		SkipDefaultTransaction: true, // Better performance for reads
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Get underlying sql.DB to configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// SQLite doesn't really use connection pooling, but set reasonable values
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConn)
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConn)
	sqlDB.SetConnMaxLifetime(time.Hour)

	// Store global reference
	DB = db

	log.Printf("Database connected: %s", dbPath)
	return db, nil
}

// Close closes the database connection.
func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}
