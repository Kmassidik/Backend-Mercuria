package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kmassidik/mercuria/internal/common/config"
	"github.com/kmassidik/mercuria/internal/common/logger"
	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
	logger *logger.Logger
}

type TxFunc func(ctx context.Context, tx *sql.Tx) error

// Connect establishes a connection to PostgreSQL
func Connect(cfg config.DatabaseConfig, log *logger.Logger) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Infof("Connected to database: %s", cfg.DBName)

	return &DB{DB: db, logger: log}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	db.logger.Info("Closing database connection")
	return db.DB.Close()
}

// Health checks database health
func (db *DB) Health(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	return db.PingContext(ctx)
}

// WithTransaction executes a function within a transaction
func (db *DB) WithTransaction(ctx context.Context, fn TxFunc) error { // <- Change fn's type
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("failed to begin transaction: %w", err)
    }

    defer func() {
        if p := recover(); p != nil {
            _ = tx.Rollback()
            panic(p) // re-throw panic after rollback
        }
    }()

    // Pass BOTH context and transaction to the function fn
    if err := fn(ctx, tx); err != nil { 
        if rbErr := tx.Rollback(); rbErr != nil {
            return fmt.Errorf("tx error: %v, rollback error: %v", err, rbErr)
        }
        return err
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("failed to commit transaction: %w", err)
    }

    return nil
}