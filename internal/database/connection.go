package database

import (
	"database/sql"
	"fmt"

	"github.com/kadirbelkuyu/DBRTS/internal/config"

	_ "github.com/lib/pq"
)

type Connection struct {
	DB     *sql.DB
	Config *config.Config
}

func NewConnection(cfg *config.Config) (*Connection, error) {
	if cfg.Database.Type != "" && cfg.Database.Type != "postgres" {
		return nil, fmt.Errorf("unsupported database type for SQL connection: %s", cfg.Database.Type)
	}

	db, err := sql.Open("postgres", cfg.GetConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("unable to reach database: %w", err)
	}

	return &Connection{
		DB:     db,
		Config: cfg,
	}, nil
}

func (c *Connection) Close() error {
	return c.DB.Close()
}

func (c *Connection) GetDatabaseName() string {
	return c.Config.Database.Database
}
