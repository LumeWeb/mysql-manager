package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.lumeweb.com/mysql-manager/internal/config"
	"go.uber.org/zap"
)

// Client provides MySQL backup locking functionality
type Client struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewClient(cfg *config.Config, logger *zap.Logger) (*Client, error) {
	dsn := fmt.Sprintf("root:%s@tcp(localhost:%d)/", cfg.MySQL.RootPassword, cfg.MySQL.Port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to create MySQL connection: %w", err)
	}

	// Basic connection settings
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	client := &Client{
		db:     db,
		logger: logger,
	}

	return client, nil
}

// FlushTables flushes tables with read lock for backup
func (c *Client) FlushTables(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, "FLUSH TABLES WITH READ LOCK")
	if err != nil {
		return fmt.Errorf("failed to flush tables: %w", err)
	}
	return nil
}

// UnlockTables releases the backup lock
func (c *Client) UnlockTables(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, "UNLOCK TABLES")
	if err != nil {
		return fmt.Errorf("failed to unlock tables: %w", err)
	}
	return nil
}

// Ping checks the database connection
func (c *Client) Ping() error {
	return c.db.Ping()
}

// GetStatus returns MySQL server status information
func (c *Client) GetStatus() (map[string]interface{}, error) {
    status := make(map[string]interface{})
    
    rows, err := c.db.Query("SHOW STATUS")
    if err != nil {
        return nil, fmt.Errorf("failed to get MySQL status: %w", err)
    }
    defer rows.Close()

    for rows.Next() {
        var name, value string
        if err := rows.Scan(&name, &value); err != nil {
            return nil, fmt.Errorf("failed to scan status row: %w", err)
        }
        status[name] = value
    }

    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("error iterating status rows: %w", err)
    }

    return status, nil
}

// Close closes the database connection 
func (c *Client) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}
