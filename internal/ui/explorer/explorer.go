package explorer

import (
	"fmt"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
)

func Run(cfg *config.Config) error {
	switch cfg.Database.Type {
	case "postgres":
		return runPostgresExplorer(cfg)
	case "mongo":
		return runMongoExplorer(cfg)
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
}
