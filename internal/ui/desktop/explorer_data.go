package desktop

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/database"
)

type pgPreview struct {
	Columns []string
	Rows    [][]string
	Raw     []map[string]string
	CTIDs   []string
}

type pgColumn struct {
	Name       string
	DataType   string
	Nullable   bool
	HasDefault bool
}

type pgFieldInput struct {
	Column     string
	Value      *string
	UseDefault bool
}

type mongoPreview struct {
	Documents []map[string]interface{}
	Count     int64
}

type pgTable struct {
	Schema string
	Name   string
}

func listPostgresTables(cfg *config.Config) ([]pgTable, error) {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := conn.DB.QueryContext(ctx, `
        SELECT table_schema, table_name
        FROM information_schema.tables
        WHERE table_type = 'BASE TABLE'
          AND table_schema NOT IN ('pg_catalog', 'information_schema')
        ORDER BY table_schema, table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []pgTable
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, err
		}
		tables = append(tables, pgTable{Schema: schema, Name: name})
	}
	return tables, rows.Err()
}

func fetchPostgresPreview(cfg *config.Config, table pgTable, filter string, limit int) (*pgPreview, error) {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	qualified := fmt.Sprintf("%s.%s", pq.QuoteIdentifier(table.Schema), pq.QuoteIdentifier(table.Name))
	queryBuilder := strings.Builder{}
	queryBuilder.WriteString("SELECT ctid::text AS __dbrts_ctid, * FROM ")
	queryBuilder.WriteString(qualified)
	if strings.TrimSpace(filter) != "" {
		queryBuilder.WriteString(" WHERE ")
		queryBuilder.WriteString(filter)
	}
	queryBuilder.WriteString(fmt.Sprintf(" LIMIT %d", limit))
	query := queryBuilder.String()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := conn.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	columnCount := len(columnNames)
	if columnCount == 0 {
		return &pgPreview{}, nil
	}

	scanTargets := make([]interface{}, columnCount)
	rawValues := make([]sql.NullString, columnCount)
	for i := range rawValues {
		scanTargets[i] = &rawValues[i]
	}

	var preview pgPreview
	displayColumns := make([]string, 0, columnCount-1)
	for _, col := range columnNames {
		if col == "__dbrts_ctid" {
			continue
		}
		displayColumns = append(displayColumns, col)
	}
	preview.Columns = displayColumns

	for rows.Next() {
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]string)
		rowSlice := make([]string, 0, len(displayColumns))
		var ctid string
		for idx, col := range columnNames {
			value := rawValues[idx]
			var text string
			if value.Valid {
				text = value.String
			}
			if col == "__dbrts_ctid" {
				ctid = text
				continue
			}
			rowMap[col] = text
			rowSlice = append(rowSlice, text)
		}
		preview.Raw = append(preview.Raw, rowMap)
		preview.Rows = append(preview.Rows, rowSlice)
		preview.CTIDs = append(preview.CTIDs, ctid)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &preview, nil
}

func loadPostgresColumns(cfg *config.Config, table pgTable) ([]pgColumn, error) {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := conn.DB.QueryContext(ctx, `
        SELECT column_name,
               data_type,
               (is_nullable = 'YES') AS nullable,
               (column_default IS NOT NULL) AS has_default
        FROM information_schema.columns
        WHERE table_schema = $1 AND table_name = $2
        ORDER BY ordinal_position`, table.Schema, table.Name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []pgColumn
	for rows.Next() {
		var column pgColumn
		if err := rows.Scan(&column.Name, &column.DataType, &column.Nullable, &column.HasDefault); err != nil {
			return nil, err
		}
		cols = append(cols, column)
	}
	return cols, rows.Err()
}

func updatePostgresRow(cfg *config.Config, table pgTable, values []pgFieldInput, ctid string) error {
	if len(values) == 0 {
		return nil
	}

	conn, err := database.NewConnection(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	setClauses := make([]string, 0, len(values))
	args := make([]interface{}, 0, len(values)+1)
	argIndex := 1
	for _, field := range values {
		if field.UseDefault {
			setClauses = append(setClauses, fmt.Sprintf("%s = DEFAULT", pq.QuoteIdentifier(field.Column)))
			continue
		}
		if field.Value == nil {
			setClauses = append(setClauses, fmt.Sprintf("%s = NULL", pq.QuoteIdentifier(field.Column)))
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", pq.QuoteIdentifier(field.Column), argIndex))
		args = append(args, field.Value)
		argIndex++
	}

	identifier := fmt.Sprintf("%s.%s", pq.QuoteIdentifier(table.Schema), pq.QuoteIdentifier(table.Name))
	stmt := fmt.Sprintf("UPDATE %s SET %s WHERE ctid = $%d::tid", identifier, strings.Join(setClauses, ", "), argIndex)
	args = append(args, ctid)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err = conn.DB.ExecContext(ctx, stmt, args...)
	return err
}

func deletePostgresRow(cfg *config.Config, table pgTable, ctid string) error {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	identifier := fmt.Sprintf("%s.%s", pq.QuoteIdentifier(table.Schema), pq.QuoteIdentifier(table.Name))
	stmt := fmt.Sprintf("DELETE FROM %s WHERE ctid = $1::tid", identifier)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err = conn.DB.ExecContext(ctx, stmt, ctid)
	return err
}

func insertPostgresRow(cfg *config.Config, table pgTable, values []pgFieldInput) error {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return err
	}
	defer conn.Close()

	identifier := fmt.Sprintf("%s.%s", pq.QuoteIdentifier(table.Schema), pq.QuoteIdentifier(table.Name))

	var columns []string
	var placeholders []string
	args := make([]interface{}, 0, len(values))
	argIndex := 1

	for _, field := range values {
		if field.UseDefault {
			continue
		}
		columns = append(columns, pq.QuoteIdentifier(field.Column))
		if field.Value == nil {
			placeholders = append(placeholders, "NULL")
		} else {
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
			args = append(args, field.Value)
			argIndex++
		}
	}

	var stmt string
	if len(columns) == 0 {
		stmt = fmt.Sprintf("INSERT INTO %s DEFAULT VALUES", identifier)
	} else {
		stmt = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", identifier, strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	_, err = conn.DB.ExecContext(ctx, stmt, args...)
	return err
}

func listMongoCollections(cfg *config.Config) ([]string, error) {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	return database.ListCollectionNames(ctx, bson.D{})
}

func fetchMongoPreview(cfg *config.Config, collection string, filter string, limit int) (*mongoPreview, error) {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.Background())

	if limit <= 0 || limit > 500 {
		limit = 50
	}

	parsedFilter, err := parseMongoFilter(filter)
	if err != nil {
		return nil, err
	}

	docs, count, err := queryMongoDocuments(database.Collection(collection), parsedFilter, limit)
	if err != nil {
		return nil, err
	}

	return &mongoPreview{Documents: docs, Count: count}, nil
}

func parseMongoFilter(filter string) (interface{}, error) {
	if strings.TrimSpace(filter) == "" {
		return bson.D{}, nil
	}
	var doc bson.M
	if err := bson.UnmarshalExtJSON([]byte(filter), true, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func queryMongoDocuments(coll *mongo.Collection, filter interface{}, limit int) ([]map[string]interface{}, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	opts := options.Find()
	opts.SetLimit(int64(limit))

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var docs []map[string]interface{}
	for cursor.Next(ctx) {
		var doc map[string]interface{}
		if err := cursor.Decode(&doc); err != nil {
			return nil, 0, err
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return nil, 0, err
	}

	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}

	return docs, count, nil
}

func replaceMongoDocument(cfg *config.Config, collection string, filter bson.M, document bson.M) error {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = database.Collection(collection).ReplaceOne(ctx, filter, document)
	return err
}

func deleteMongoDocument(cfg *config.Config, collection string, filter bson.M) error {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = database.Collection(collection).DeleteOne(ctx, filter)
	return err
}

func insertMongoDocument(cfg *config.Config, collection string, doc bson.M) error {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = database.Collection(collection).InsertOne(ctx, doc)
	return err
}

func connectMongoDatabase(cfg *config.Config) (*mongo.Client, *mongo.Database, error) {
	if strings.TrimSpace(cfg.Database.Database) == "" {
		return nil, nil, fmt.Errorf("database name is required for MongoDB exploration")
	}

	uri := cfg.GetMongoURI()
	if strings.TrimSpace(uri) == "" {
		return nil, nil, fmt.Errorf("mongo URI could not be derived from config")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(context.Background())
		return nil, nil, err
	}

	return client, client.Database(cfg.Database.Database), nil
}

func formatJSON(doc map[string]interface{}) string {
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Sprintf("<error: %v>", err)
	}
	return string(payload)
}

func summarizeDocument(doc map[string]interface{}) string {
	if id, ok := doc["_id"]; ok {
		return fmt.Sprintf("_id=%v", id)
	}
	for key, value := range doc {
		return fmt.Sprintf("%s=%v", key, value)
	}
	return "Document"
}

func parseLimitInput(value string, fallback int) int {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func runPostgresStatement(cfg *config.Config, statement string, limit int) ([]string, [][]string, string, error) {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return nil, nil, "", err
	}
	defer conn.Close()

	stmt := strings.TrimSpace(statement)
	if stmt == "" {
		return nil, nil, "", fmt.Errorf("SQL cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if looksLikeQuery(stmt) {
		rows, err := conn.DB.QueryContext(ctx, stmt)
		if err != nil {
			return nil, nil, "", err
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			return nil, nil, "", err
		}
		if limit <= 0 || limit > 1000 {
			limit = 200
		}

		columnCount := len(columns)
		raw := make([]sql.NullString, columnCount)
		dest := make([]interface{}, columnCount)
		for i := range raw {
			dest[i] = &raw[i]
		}

		var data [][]string
		for rows.Next() {
			if err := rows.Scan(dest...); err != nil {
				return nil, nil, "", err
			}
			row := make([]string, columnCount)
			for i := range columns {
				if raw[i].Valid {
					row[i] = raw[i].String
				} else {
					row[i] = "NULL"
				}
			}
			data = append(data, row)
			if len(data) >= limit {
				break
			}
		}
		if err := rows.Err(); err != nil {
			return nil, nil, "", err
		}

		message := fmt.Sprintf("%d row(s) fetched.", len(data))
		return columns, data, message, nil
	}

	result, err := conn.DB.ExecContext(ctx, stmt)
	if err != nil {
		return nil, nil, "", err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, nil, "Command executed.", nil
	}
	return nil, nil, fmt.Sprintf("%d row(s) affected.", affected), nil
}

func looksLikeQuery(stmt string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(stmt))
	if trimmed == "" {
		return false
	}
	keywords := []string{"select", "with", "show", "describe", "explain"}
	for _, keyword := range keywords {
		if strings.HasPrefix(trimmed, keyword) {
			return true
		}
	}
	return false
}

func runMongoCollectionCommand(cfg *config.Config, collection, command string, limit int) (string, string, bool, error) {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return "", "", false, err
	}
	defer client.Disconnect(context.Background())

	action, payload := splitMongoCommand(command)
	if action == "" {
		return "", "", false, fmt.Errorf("command is required")
	}

	coll := database.Collection(collection)

	switch action {
	case "insert":
		doc, err := decodeMongoDocument(payload)
		if err != nil {
			return "", "", false, fmt.Errorf("invalid insert payload: %w", err)
		}
		if len(doc) == 0 {
			return "", "", false, fmt.Errorf("insert payload cannot be empty")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := coll.InsertOne(ctx, doc)
		if err != nil {
			return "", "", false, err
		}
		return "", fmt.Sprintf("Inserted document with ID %v.", result.InsertedID), true, nil
	case "update":
		filter, update, err := parseMongoUpdatePayload(payload)
		if err != nil {
			return "", "", false, fmt.Errorf("invalid update payload: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := coll.UpdateMany(ctx, filter, update)
		if err != nil {
			return "", "", false, err
		}
		return "", fmt.Sprintf("Matched %d • Modified %d.", result.MatchedCount, result.ModifiedCount), true, nil
	case "delete":
		filter, err := decodeMongoDocument(payload)
		if err != nil {
			return "", "", false, fmt.Errorf("invalid delete payload: %w", err)
		}
		if len(filter) == 0 {
			return "", "", false, fmt.Errorf("delete filter is required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := coll.DeleteMany(ctx, filter)
		if err != nil {
			return "", "", false, err
		}
		return "", fmt.Sprintf("Deleted %d document(s).", result.DeletedCount), true, nil
	case "find":
		filter, err := decodeMongoDocument(payload)
		if err != nil {
			return "", "", false, fmt.Errorf("invalid find payload: %w", err)
		}
		docs, count, err := queryMongoDocuments(coll, filter, limit)
		if err != nil {
			return "", "", false, err
		}
		text := formatMongoDocuments(docs)
		return text, fmt.Sprintf("%d document(s) matched.", count), false, nil
	default:
		return "", "", false, fmt.Errorf("unsupported command: %s", action)
	}
}

func splitMongoCommand(input string) (string, string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", ""
	}
	parts := strings.SplitN(trimmed, " ", 2)
	action := strings.ToLower(parts[0])
	if len(parts) == 1 {
		return action, ""
	}
	return action, strings.TrimSpace(parts[1])
}

func decodeMongoDocument(payload string) (bson.M, error) {
	if strings.TrimSpace(payload) == "" {
		return bson.M{}, nil
	}
	var doc bson.M
	if err := bson.UnmarshalExtJSON([]byte(payload), true, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

type mongoUpdatePayload struct {
	Filter bson.M `bson:"filter" json:"filter"`
	Update bson.M `bson:"update" json:"update"`
}

func parseMongoUpdatePayload(payload string) (bson.M, bson.M, error) {
	var data mongoUpdatePayload
	if err := bson.UnmarshalExtJSON([]byte(payload), true, &data); err != nil {
		return nil, nil, err
	}
	if data.Update == nil || len(data.Update) == 0 {
		return nil, nil, fmt.Errorf("update document is required")
	}
	if data.Filter == nil {
		data.Filter = bson.M{}
	}
	return data.Filter, data.Update, nil
}

func formatMongoDocuments(docs []map[string]interface{}) string {
	if len(docs) == 0 {
		return "No documents found."
	}
	var builder strings.Builder
	for i, doc := range docs {
		payload, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			builder.WriteString(fmt.Sprintf("<error: %v>", err))
		} else {
			builder.Write(payload)
		}
		if i < len(docs)-1 {
			builder.WriteString("\n---\n")
		}
	}
	return builder.String()
}
