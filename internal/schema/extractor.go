package schema

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/kadirbelkuyu/DBRTS/internal/database"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type Extractor struct {
	conn   *database.Connection
	logger *logger.Logger
}

func NewExtractor(conn *database.Connection, logger *logger.Logger) *Extractor {
	return &Extractor{
		conn:   conn,
		logger: logger,
	}
}

func (e *Extractor) ExtractTables(schemaFilter string) ([]Table, error) {
	e.logger.Info("Extracting tables...")

	query := `
		SELECT 
			t.table_name,
			t.table_schema
		FROM information_schema.tables t
		WHERE t.table_type = 'BASE TABLE'
		AND t.table_schema NOT IN ('information_schema', 'pg_catalog', 'pg_toast')
	`

	if schemaFilter != "" {
		query += fmt.Sprintf(" AND t.table_schema = '%s'", schemaFilter)
	}

	query += " ORDER BY t.table_schema, t.table_name"

	rows, err := e.conn.DB.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var table Table
		if err := rows.Scan(&table.Name, &table.Schema); err != nil {
			return nil, fmt.Errorf("failed to read table metadata: %w", err)
		}

		if err := e.extractTableDetails(&table); err != nil {
			return nil, fmt.Errorf("failed to gather table details for %s.%s: %w", table.Schema, table.Name, err)
		}

		tables = append(tables, table)
	}

	e.logger.Infof("%d tables extracted", len(tables))
	return tables, nil
}

func (e *Extractor) extractTableDetails(table *Table) error {
	if err := e.extractColumns(table); err != nil {
		return err
	}

	if err := e.extractPrimaryKeys(table); err != nil {
		return err
	}

	if err := e.extractForeignKeys(table); err != nil {
		return err
	}

	if err := e.extractIndexes(table); err != nil {
		return err
	}

	if err := e.extractRowCount(table); err != nil {
		return err
	}

	return nil
}

func (e *Extractor) extractColumns(table *Table) error {
	query := `
		SELECT 
			column_name,
			data_type,
			is_nullable,
			column_default,
			character_maximum_length,
			ordinal_position
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`

	rows, err := e.conn.DB.Query(query, table.Schema, table.Name)
	if err != nil {
		return fmt.Errorf("failed to query column metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var col Column
		var isNullable string
		var defaultValue sql.NullString
		var maxLength sql.NullInt64

		err := rows.Scan(
			&col.Name,
			&col.DataType,
			&isNullable,
			&defaultValue,
			&maxLength,
			&col.Position,
		)
		if err != nil {
			return fmt.Errorf("failed to read column metadata: %w", err)
		}

		col.IsNullable = isNullable == "YES"
		if defaultValue.Valid {
			col.DefaultValue = &defaultValue.String
		}
		if maxLength.Valid {
			length := int(maxLength.Int64)
			col.MaxLength = &length
		}

		table.Columns = append(table.Columns, col)
	}

	return nil
}

func (e *Extractor) extractPrimaryKeys(table *Table) error {
	query := `
		SELECT column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = $1 AND table_name = $2
		AND constraint_name IN (
			SELECT constraint_name
			FROM information_schema.table_constraints
			WHERE table_schema = $1 AND table_name = $2
			AND constraint_type = 'PRIMARY KEY'
		)
		ORDER BY ordinal_position
	`

	rows, err := e.conn.DB.Query(query, table.Schema, table.Name)
	if err != nil {
		return fmt.Errorf("failed to query primary key metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var columnName string
		if err := rows.Scan(&columnName); err != nil {
			return fmt.Errorf("failed to read primary key metadata: %w", err)
		}
		table.PrimaryKeys = append(table.PrimaryKeys, columnName)
	}

	return nil
}

func (e *Extractor) extractForeignKeys(table *Table) error {
	query := `
		SELECT 
			tc.constraint_name,
			kcu.column_name,
			ccu.table_name AS foreign_table_name,
			ccu.column_name AS foreign_column_name,
			ccu.table_schema AS foreign_table_schema,
			rc.delete_rule,
			rc.update_rule
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
			ON ccu.constraint_name = tc.constraint_name
			AND ccu.table_schema = tc.table_schema
		JOIN information_schema.referential_constraints AS rc
			ON tc.constraint_name = rc.constraint_name
			AND tc.table_schema = rc.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
		AND tc.table_schema = $1 AND tc.table_name = $2
	`

	rows, err := e.conn.DB.Query(query, table.Schema, table.Name)
	if err != nil {
		return fmt.Errorf("failed to query foreign key metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var fk ForeignKey
		err := rows.Scan(
			&fk.Name,
			&fk.ColumnName,
			&fk.ReferencedTable,
			&fk.ReferencedColumn,
			&fk.ReferencedSchema,
			&fk.OnDelete,
			&fk.OnUpdate,
		)
		if err != nil {
			return fmt.Errorf("failed to read foreign key metadata: %w", err)
		}
		table.ForeignKeys = append(table.ForeignKeys, fk)
	}

	return nil
}

func (e *Extractor) extractIndexes(table *Table) error {
	query := `
		SELECT 
			i.indexname,
			i.tablename,
			pg_get_indexdef(ix.indexrelid) as indexdef,
			ix.indisunique,
			ix.indisprimary
		FROM pg_indexes i
		JOIN pg_class c ON c.relname = i.tablename
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_index ix ON ix.indexrelid = (
			SELECT oid FROM pg_class WHERE relname = i.indexname
		)
		WHERE n.nspname = $1 AND i.tablename = $2
	`

	rows, err := e.conn.DB.Query(query, table.Schema, table.Name)
	if err != nil {
		return fmt.Errorf("failed to query index metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var idx Index
		var indexDef string
		err := rows.Scan(
			&idx.Name,
			&idx.TableName,
			&indexDef,
			&idx.IsUnique,
			&idx.IsPrimary,
		)
		if err != nil {
			return fmt.Errorf("failed to read index metadata: %w", err)
		}

		idx.Columns = e.parseIndexColumns(indexDef)
		idx.IndexType = e.parseIndexType(indexDef)

		table.Indexes = append(table.Indexes, idx)
	}

	return nil
}

func (e *Extractor) extractRowCount(table *Table) error {
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", table.Schema, table.Name)

	if err := e.conn.DB.QueryRow(query).Scan(&table.RowCount); err != nil {
		return fmt.Errorf("failed to query row count: %w", err)
	}

	return nil
}

func (e *Extractor) parseIndexColumns(indexDef string) []string {
	start := strings.Index(indexDef, "(")
	end := strings.Index(indexDef, ")")
	if start == -1 || end == -1 {
		return []string{}
	}

	columnsPart := indexDef[start+1 : end]
	columns := strings.Split(columnsPart, ",")

	for i, col := range columns {
		columns[i] = strings.TrimSpace(col)
	}

	return columns
}

func (e *Extractor) parseIndexType(indexDef string) string {
	if strings.Contains(strings.ToUpper(indexDef), "USING BTREE") {
		return "BTREE"
	}
	if strings.Contains(strings.ToUpper(indexDef), "USING HASH") {
		return "HASH"
	}
	if strings.Contains(strings.ToUpper(indexDef), "USING GIN") {
		return "GIN"
	}
	if strings.Contains(strings.ToUpper(indexDef), "USING GIST") {
		return "GIST"
	}
	return "BTREE"
}
