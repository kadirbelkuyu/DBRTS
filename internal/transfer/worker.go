package transfer

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/kadirbelkuyu/DBRTS/internal/database"
	"github.com/kadirbelkuyu/DBRTS/internal/schema"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
	"github.com/kadirbelkuyu/DBRTS/pkg/progress"
)

type WorkerPool struct {
	workers   int
	batchSize int
	jobs      chan Job
}

type Job interface {
	Execute() error
}

type DataTransferJob struct {
	Table       schema.Table
	SourceConn  *database.Connection
	TargetConn  *database.Connection
	BatchSize   int
	ProgressBar *progress.Bar
	Logger      *logger.Logger
}

func NewWorkerPool(workers, batchSize int) *WorkerPool {
	return &WorkerPool{
		workers:   workers,
		batchSize: batchSize,
		jobs:      make(chan Job, workers*2),
	}
}

func (wp *WorkerPool) SubmitJob(ctx context.Context, job Job) error {
	select {
	case wp.jobs <- job:
		return job.Execute()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (dt *DataTransferJob) Execute() error {
	dt.Logger.Logger.Infof("Starting table transfer: %s.%s (%d rows)", dt.Table.Schema, dt.Table.Name, dt.Table.RowCount)

	offset := int64(0)
	batchSize := int64(dt.BatchSize)

	for offset < dt.Table.RowCount {
		limit := batchSize
		if offset+limit > dt.Table.RowCount {
			limit = dt.Table.RowCount - offset
		}

		if err := dt.transferBatch(offset, limit); err != nil {
			return fmt.Errorf("batch transfer failed: %w", err)
		}

		dt.ProgressBar.IncrementBy(limit)
		offset += limit
	}

	dt.Logger.Logger.Infof("Table transfer completed: %s.%s", dt.Table.Schema, dt.Table.Name)
	return nil
}

func (dt *DataTransferJob) transferBatch(offset, limit int64) error {
	selectQuery := dt.buildSelectQuery(offset, limit)

	rows, err := dt.SourceConn.DB.Query(selectQuery)
	if err != nil {
		return fmt.Errorf("failed to query source data: %w", err)
	}
	defer rows.Close()

	insertQuery := dt.buildInsertQuery()

	tx, err := dt.TargetConn.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(insertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to fetch column metadata: %w", err)
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))

		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		if _, err := stmt.Exec(values...); err != nil {
			return fmt.Errorf("failed to insert row: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (dt *DataTransferJob) buildSelectQuery(offset, limit int64) string {
	columnNames := make([]string, len(dt.Table.Columns))
	for i, col := range dt.Table.Columns {
		columnNames[i] = fmt.Sprintf(`"%s"`, col.Name)
	}

	return fmt.Sprintf(
		`SELECT %s FROM "%s"."%s" ORDER BY %s OFFSET %d LIMIT %d`,
		strings.Join(columnNames, ", "),
		dt.Table.Schema,
		dt.Table.Name,
		dt.buildOrderByClause(),
		offset,
		limit,
	)
}

func (dt *DataTransferJob) buildInsertQuery() string {
	columnNames := make([]string, len(dt.Table.Columns))
	placeholders := make([]string, len(dt.Table.Columns))

	for i, col := range dt.Table.Columns {
		columnNames[i] = fmt.Sprintf(`"%s"`, col.Name)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	return fmt.Sprintf(
		`INSERT INTO "%s"."%s" (%s) VALUES (%s) ON CONFLICT DO NOTHING`,
		dt.Table.Schema,
		dt.Table.Name,
		strings.Join(columnNames, ", "),
		strings.Join(placeholders, ", "),
	)
}

func (dt *DataTransferJob) buildOrderByClause() string {
	if len(dt.Table.PrimaryKeys) > 0 {
		pkCols := make([]string, len(dt.Table.PrimaryKeys))
		for i, pk := range dt.Table.PrimaryKeys {
			pkCols[i] = fmt.Sprintf(`"%s"`, pk)
		}
		return strings.Join(pkCols, ", ")
	}

	if len(dt.Table.Columns) > 0 {
		return fmt.Sprintf(`"%s"`, dt.Table.Columns[0].Name)
	}

	return "1"
}

func convertValue(value interface{}, dataType string) interface{} {
	if value == nil {
		return nil
	}

	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}

	switch dataType {
	case "bytea":
		if bytes, ok := value.([]byte); ok {
			return bytes
		}
	case "json", "jsonb":
		if str, ok := value.(string); ok {
			return str
		}
		if bytes, ok := value.([]byte); ok {
			return string(bytes)
		}
	}

	return value
}
