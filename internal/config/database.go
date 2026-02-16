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

// RunMigrations runs auto-migrations to ensure DB schema is up to date
func RunMigrations(pool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrations := []string{
		// Add qris.pw related columns to payments table
		`ALTER TABLE payments ADD COLUMN IF NOT EXISTS qr_image_url TEXT DEFAULT ''`,
		`ALTER TABLE payments ADD COLUMN IF NOT EXISTS qrispw_transaction_id TEXT DEFAULT ''`,
	}

	for _, m := range migrations {
		if _, err := pool.Exec(ctx, m); err != nil {
			log.Printf("⚠️  Migration warning: %v", err)
		}
	}

	log.Println("✅ Database migrations completed")
}
