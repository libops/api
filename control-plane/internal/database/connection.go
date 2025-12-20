package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Config holds database configuration
type Config struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// DefaultConfig returns default database configuration
func DefaultConfig() Config {
	return Config{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// NewPool creates a new database connection pool
func NewPool(databaseURL string, cfg Config) (*sql.DB, error) {
	database, err := sql.Open("mysql", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	database.SetMaxOpenConns(cfg.MaxOpenConns)
	database.SetMaxIdleConns(cfg.MaxIdleConns)
	database.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := database.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return database, nil
}

// NewConnection creates a new database connection (legacy wrapper)
func NewConnection(ctx context.Context, databaseURL string) (*sql.DB, error) {
	return NewPool(databaseURL, DefaultConfig())
}
