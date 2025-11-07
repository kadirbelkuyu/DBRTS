package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/kadirbelkuyu/DBRTS/internal/backup"
	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/transfer"
	"github.com/kadirbelkuyu/DBRTS/pkg/interactive"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) Transfer(sourceCfg, targetCfg *config.Config, schemaOnly, dataOnly bool, workers, batch int, verboseFlag bool) error {
	if schemaOnly && dataOnly {
		fmt.Println("Both schema-only and data-only were selected. Running a full transfer instead.")
		schemaOnly = false
		dataOnly = false
	}

	log := logger.NewLogger(verboseFlag)
	log.Logger.Info("Starting data transfer...")

	opts := transfer.Options{
		SchemaOnly:      schemaOnly,
		DataOnly:        dataOnly,
		ParallelWorkers: workers,
		BatchSize:       batch,
		Logger:          log,
	}

	service, err := transfer.NewService(sourceCfg, targetCfg, opts)
	if err != nil {
		return fmt.Errorf("failed to initialize transfer service: %w", err)
	}

	if err := service.Execute(); err != nil {
		return fmt.Errorf("transfer execution failed: %w", err)
	}

	log.Logger.Info("Data transfer completed successfully!")
	return nil
}

func (s *Service) Backup(cfg *config.Config, verboseFlag bool) error {
	log := logger.NewLogger(verboseFlag)
	log.Logger.Info("Starting backup...")

	service, err := backup.NewService(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to initialize backup service: %w", err)
	}
	if err := service.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer service.Close()

	databases, err := service.ListDatabases()
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}

	selector := interactive.NewDatabaseSelector(cfg.Database.Type)
	selected, err := selector.SelectDatabase(databases)
	if err != nil {
		return fmt.Errorf("database selection failed: %w", err)
	}

	if !selector.ConfirmAction("Backup", selected.Name) {
		log.Logger.Info("Operation cancelled by user.")
		return nil
	}

	options := selector.GetBackupOptions(cfg.Database.Type)

	metadata, err := service.CreateBackup(selected.Name, options)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Println()
	fmt.Println("Backup completed successfully.")
	fmt.Printf("File: %s\n", metadata.Location)
	fmt.Printf("Size: %d bytes\n", metadata.BackupSize)
	fmt.Printf("Checksum: %s\n", shortChecksum(metadata.Checksum))
	fmt.Printf("Duration: %s\n", metadata.CompletedAt.Sub(metadata.StartedAt).Round(time.Second))

	return nil
}

func (s *Service) Restore(cfg *config.Config, verboseFlag bool) error {
	log := logger.NewLogger(verboseFlag)
	log.Logger.Info("Starting restore...")

	service, err := backup.NewService(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to initialize backup service: %w", err)
	}
	if err := service.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer service.Close()

	selector := interactive.NewDatabaseSelector(cfg.Database.Type)
	options := selector.GetRestoreOptions(cfg.Database.Type)

	if !selector.ConfirmAction("Restore", options.TargetDatabase) {
		log.Logger.Info("Operation cancelled by user.")
		return nil
	}

	if err := service.RestoreBackup(options); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}

	fmt.Println()
	fmt.Println("Restore completed successfully.")
	return nil
}

func (s *Service) ListDatabases(cfg *config.Config) error {
	log := logger.NewLogger(false)
	service, err := backup.NewService(cfg, log)
	if err != nil {
		return fmt.Errorf("failed to initialize backup service: %w", err)
	}
	if err := service.Connect(); err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer service.Close()

	databases, err := service.ListDatabases()
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}

	target := formatServerLabel(cfg)
	fmt.Printf("\nDatabases on %s (%s):\n", target, cfg.Database.Type)
	fmt.Println(strings.Repeat("=", 36))
	for i, db := range databases {
		if cfg.Database.Type == "postgres" {
			fmt.Printf("%d. %s (Owner: %s, Size: %s)\n",
				i+1,
				db.Name,
				displayValue(db.Owner, "n/a"),
				displayValue(db.Size, "n/a"),
			)
		} else {
			fmt.Printf("%d. %s (Collections: %d, Size: %s)\n",
				i+1,
				db.Name,
				db.Collections,
				displayValue(db.Size, "n/a"),
			)
		}
	}
	fmt.Printf("\nTotal databases: %d\n", len(databases))
	return nil
}

func shortChecksum(checksum string) string {
	if len(checksum) <= 16 {
		return checksum
	}
	return checksum[:16] + "..."
}

func formatServerLabel(cfg *config.Config) string {
	host := strings.TrimSpace(cfg.Database.Host)
	if host == "" {
		if cfg.Database.URI != "" {
			return cfg.Database.URI
		}
		host = "localhost"
	}

	if cfg.Database.Port > 0 {
		return fmt.Sprintf("%s:%d", host, cfg.Database.Port)
	}

	return host
}

func displayValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
