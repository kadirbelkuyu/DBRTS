package schema

import (
	"fmt"
	"strings"

	"github.com/kadirbelkuyu/DBRTS/internal/database"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type Creator struct {
	conn   *database.Connection
	logger *logger.Logger
}

func NewCreator(conn *database.Connection, logger *logger.Logger) *Creator {
	return &Creator{
		conn:   conn,
		logger: logger,
	}
}

func (c *Creator) CreateTables(tables []Table) error {
	c.logger.Logger.Info("Creating tables...")

	tx, err := c.conn.DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, table := range tables {
		if err := c.createTable(tx, table); err != nil {
			return fmt.Errorf("failed to create table %s.%s: %w", table.Schema, table.Name, err)
		}
	}

	for _, table := range tables {
		if err := c.createIndexes(tx, table); err != nil {
			return fmt.Errorf("failed to create indexes for %s.%s: %w", table.Schema, table.Name, err)
		}
	}

	for _, table := range tables {
		if err := c.createForeignKeys(tx, table); err != nil {
			return fmt.Errorf("failed to create foreign keys for %s.%s: %w", table.Schema, table.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	c.logger.Logger.Infof("%d tables created successfully", len(tables))
	return nil
}

func (c *Creator) createTable(tx interface{}, table Table) error {
	var columnDefs []string

	for _, col := range table.Columns {
		colDef := fmt.Sprintf(`"%s" %s`, col.Name, col.DataType)

		if col.MaxLength != nil && (col.DataType == "character varying" || col.DataType == "varchar") {
			colDef = fmt.Sprintf(`"%s" %s(%d)`, col.Name, col.DataType, *col.MaxLength)
		}

		if !col.IsNullable {
			colDef += " NOT NULL"
		}

		if col.DefaultValue != nil {
			colDef += fmt.Sprintf(" DEFAULT %s", *col.DefaultValue)
		}

		columnDefs = append(columnDefs, colDef)
	}

	if len(table.PrimaryKeys) > 0 {
		pkCols := make([]string, len(table.PrimaryKeys))
		for i, pk := range table.PrimaryKeys {
			pkCols[i] = fmt.Sprintf(`"%s"`, pk)
		}
		columnDefs = append(columnDefs, fmt.Sprintf("PRIMARY KEY (%s)", strings.Join(pkCols, ", ")))
	}

	createSQL := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS "%s"."%s" (%s)`,
		table.Schema,
		table.Name,
		strings.Join(columnDefs, ", "),
	)

	c.logger.Logger.Debugf("Creating table: %s", createSQL)

	if execer, ok := tx.(interface {
		Exec(string, ...interface{}) error
	}); ok {
		return execer.Exec(createSQL)
	}

	return fmt.Errorf("transaction does not support Exec")
}

func (c *Creator) createIndexes(tx interface{}, table Table) error {
	for _, idx := range table.Indexes {
		if idx.IsPrimary {
			continue
		}

		var indexSQL string
		uniqueStr := ""
		if idx.IsUnique {
			uniqueStr = "UNIQUE "
		}

		indexCols := make([]string, len(idx.Columns))
		for i, col := range idx.Columns {
			indexCols[i] = fmt.Sprintf(`"%s"`, col)
		}

		indexSQL = fmt.Sprintf(
			`CREATE %sINDEX IF NOT EXISTS "%s" ON "%s"."%s" USING %s (%s)`,
			uniqueStr,
			idx.Name,
			table.Schema,
			table.Name,
			idx.IndexType,
			strings.Join(indexCols, ", "),
		)

		c.logger.Logger.Debugf("Creating index: %s", indexSQL)

		if execer, ok := tx.(interface {
			Exec(string, ...interface{}) error
		}); ok {
			if err := execer.Exec(indexSQL); err != nil {
				c.logger.Logger.Warnf("Failed to create index %s: %v", idx.Name, err)
			}
		}
	}

	return nil
}

func (c *Creator) createForeignKeys(tx interface{}, table Table) error {
	for _, fk := range table.ForeignKeys {
		fkSQL := fmt.Sprintf(
			`ALTER TABLE "%s"."%s" ADD CONSTRAINT "%s" FOREIGN KEY ("%s") REFERENCES "%s"."%s" ("%s")`,
			table.Schema,
			table.Name,
			fk.Name,
			fk.ColumnName,
			fk.ReferencedSchema,
			fk.ReferencedTable,
			fk.ReferencedColumn,
		)

		if fk.OnDelete != "" && fk.OnDelete != "NO ACTION" {
			fkSQL += fmt.Sprintf(" ON DELETE %s", fk.OnDelete)
		}

		if fk.OnUpdate != "" && fk.OnUpdate != "NO ACTION" {
			fkSQL += fmt.Sprintf(" ON UPDATE %s", fk.OnUpdate)
		}

		c.logger.Logger.Debugf("Creating foreign key: %s", fkSQL)

		if execer, ok := tx.(interface {
			Exec(string, ...interface{}) error
		}); ok {
			if err := execer.Exec(fkSQL); err != nil {
				c.logger.Logger.Warnf("Failed to create foreign key %s: %v", fk.Name, err)
			}
		}
	}

	return nil
}
