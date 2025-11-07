package transfer

import (
	"context"
	"fmt"
	"sync"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/database"
	"github.com/kadirbelkuyu/DBRTS/internal/schema"
	"github.com/kadirbelkuyu/DBRTS/pkg/progress"
)

type postgresEngine struct {
	sourceConfig *config.Config
	targetConfig *config.Config
	options      Options
	sourceConn   *database.Connection
	targetConn   *database.Connection
}

func newPostgresEngine(sourceConfig, targetConfig *config.Config, options Options) *postgresEngine {
	return &postgresEngine{
		sourceConfig: sourceConfig,
		targetConfig: targetConfig,
		options:      options,
	}
}

func (e *postgresEngine) Execute() error {
	e.options.Logger.Info("Starting PostgreSQL transfer...")

	if err := e.connect(); err != nil {
		return fmt.Errorf("connection error: %w", err)
	}
	defer e.cleanup()

	if !e.options.DataOnly {
		if err := e.transferSchema(); err != nil {
			return fmt.Errorf("schema transfer failed: %w", err)
		}
	}

	if !e.options.SchemaOnly {
		if err := e.transferData(); err != nil {
			return fmt.Errorf("data transfer failed: %w", err)
		}
	}

	e.options.Logger.Info("PostgreSQL transfer completed successfully.")
	return nil
}

func (e *postgresEngine) connect() error {
	e.options.Logger.Info("Connecting to source PostgreSQL database...")
	sourceConn, err := database.NewConnection(e.sourceConfig)
	if err != nil {
		return fmt.Errorf("source database connection: %w", err)
	}
	e.sourceConn = sourceConn

	e.options.Logger.Info("Connecting to target PostgreSQL database...")
	targetConn, err := database.NewConnection(e.targetConfig)
	if err != nil {
		return fmt.Errorf("target database connection: %w", err)
	}
	e.targetConn = targetConn

	return nil
}

func (e *postgresEngine) cleanup() {
	if e.sourceConn != nil {
		e.sourceConn.Close()
	}
	if e.targetConn != nil {
		e.targetConn.Close()
	}
}

func (e *postgresEngine) transferSchema() error {
	e.options.Logger.Info("Transferring schema...")

	extractor := schema.NewExtractor(e.sourceConn, e.options.Logger)
	creator := schema.NewCreator(e.targetConn, e.options.Logger)

	tables, err := extractor.ExtractTables("")
	if err != nil {
		return fmt.Errorf("failed to extract tables: %w", err)
	}

	if err := creator.CreateTables(tables); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	e.options.Logger.Info("Schema transfer completed.")
	return nil
}

func (e *postgresEngine) transferData() error {
	e.options.Logger.Info("Transferring data...")

	extractor := schema.NewExtractor(e.sourceConn, e.options.Logger)
	tables, err := extractor.ExtractTables("")
	if err != nil {
		return fmt.Errorf("failed to extract table metadata: %w", err)
	}

	totalRows := int64(0)
	for _, table := range tables {
		totalRows += table.RowCount
	}

	progressBar := progress.NewBar(totalRows, "Data transfer")

	ctx := context.Background()
	workerPool := NewWorkerPool(e.options.ParallelWorkers, e.options.BatchSize)

	var wg sync.WaitGroup
	for _, table := range tables {
		if table.RowCount == 0 {
			continue
		}

		wg.Add(1)
		go func(t schema.Table) {
			defer wg.Done()

			job := &DataTransferJob{
				Table:       t,
				SourceConn:  e.sourceConn,
				TargetConn:  e.targetConn,
				BatchSize:   e.options.BatchSize,
				ProgressBar: progressBar,
				Logger:      e.options.Logger,
			}

			if err := workerPool.SubmitJob(ctx, job); err != nil {
				e.options.Logger.Errorf("Table transfer failed for %s: %v", t.Name, err)
			}
		}(table)
	}

	wg.Wait()
	progressBar.Finish()

	e.options.Logger.Info("Data transfer completed.")
	return nil
}
