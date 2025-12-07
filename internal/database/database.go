// Package database provides utilities for database connection management.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// Config holds database connection pool configuration.
type Config struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
	PingTimeout     time.Duration
}

// DefaultConfig returns sensible defaults for production use.
func DefaultConfig() *Config {
	return &Config{
		MaxOpenConns:    25,
		MaxIdleConns:    25,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
		PingTimeout:     5 * time.Second,
	}
}

// NewPool creates a production-ready database connection pool.
func NewPool(connStr string, cfg *Config) (*sql.DB, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// sql.Open does not establish any connections, it just prepares the pool
	dbPool, err := sql.Open("mysql", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	dbPool.SetMaxOpenConns(cfg.MaxOpenConns)
	dbPool.SetMaxIdleConns(cfg.MaxIdleConns)
	dbPool.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	dbPool.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.PingTimeout)
	defer cancel()

	if err = dbPool.PingContext(ctx); err != nil {
		_ = dbPool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return dbPool, nil
}
