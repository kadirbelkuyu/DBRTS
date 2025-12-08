package backup

import (
	"fmt"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type Service interface {
	Connect() error
	Close() error
	ListDatabases() ([]DatabaseInfo, error)
	CreateBackup(database string, options BackupOptions) (*BackupMetadata, error)
	RestoreBackup(options RestoreOptions) error
}

func NewService(cfg *config.Config, log *logger.Logger) (Service, error) {
	switch cfg.Database.Type {
	case "postgres":
		return newPostgresService(cfg, log), nil
	case "mongo":
		return newMongoService(cfg, log), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
}
