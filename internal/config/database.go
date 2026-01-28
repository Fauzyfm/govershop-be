package config

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is the global database connection pool
var DB *pgxpool.Pool

// ConnectDB establishes connection to PostgreSQL database
func ConnectDB(databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}

	// Connection pool settings
	config.MaxConns = 25
	config.MinConns = 5
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = 30 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	log.Println("✅ Connected to PostgreSQL database")
	DB = pool
	return pool, nil
}

// CloseDB closes the database connection pool
func CloseDB() {
	if DB != nil {
		DB.Close()
		log.Println("Database connection closed")
	}
}
