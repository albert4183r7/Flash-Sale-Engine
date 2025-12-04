package postgres

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// Client wraps the PostgreSQL connection
type Client struct {
	db *sql.DB
}

// NewClient creates a new PostgreSQL client
func NewClient(connectionURL string) (*Client, error) {
	var db *sql.DB
	var err error

	for i := 0; i < 5; i++ {
		db, err = sql.Open("postgres", connectionURL)
		if err == nil {
			err = db.Ping()
			if err == nil {
				break
			}
		}
		log.Printf("Failed to connect to PostgreSQL (attempt %d/5): %v", i+1, err)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL after 5 attempts: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	log.Println("Connected to PostgreSQL successfully")
	return &Client{db: db}, nil
}

// Close closes the database connection
func (c *Client) Close() error {
	return c.db.Close()
}

// DB returns the underlying database connection
func (c *Client) DB() *sql.DB {
	return c.db
}

// InitializeSchema creates the orders table if it doesn't exist
func (c *Client) InitializeSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS orders (
		id UUID PRIMARY KEY,
		user_id UUID NOT NULL REFERENCES users(id),   
		product_id UUID NOT NULL REFERENCES products(id),
		qty INT NOT NULL,
		status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	);

		CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
		CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
	`

	_, err := c.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("Database schema initialized successfully")
	return nil
}
