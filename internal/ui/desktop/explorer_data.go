package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/database"
)

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

func fetchPostgresPreview(cfg *config.Config, table pgTable, limit int) ([]string, [][]string, error) {
	conn, err := database.NewConnection(cfg)
	if err != nil {
		return nil, nil, err
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 200
	}

	qualified := fmt.Sprintf("%s.%s", pq.QuoteIdentifier(table.Schema), pq.QuoteIdentifier(table.Name))
	query := fmt.Sprintf("SELECT * FROM %s LIMIT %d", qualified, limit)
	rows, err := conn.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var data [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(columns))
		for i, val := range values {
			row[i] = renderValue(val)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return columns, data, nil
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
		if limit <= 0 {
			limit = 200
		}
		var data [][]string
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, nil, "", err
			}
			row := make([]string, len(columns))
			for i, val := range values {
				row[i] = renderValue(val)
			}
			data = append(data, row)
			if len(data) >= limit {
				break
			}
		}
		if err := rows.Err(); err != nil {
			return nil, nil, "", err
		}
		msg := fmt.Sprintf("%d row(s) fetched.", len(data))
		return columns, data, msg, nil
	}

	result, err := conn.DB.ExecContext(ctx, stmt)
	if err != nil {
		return nil, nil, "", err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, nil, "", nil
	}
	return nil, nil, fmt.Sprintf("%d row(s) affected.", count), nil
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

func renderValue(value interface{}) string {
	if value == nil {
		return "NULL"
	}
	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return fmt.Sprint(v)
	}
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

func fetchMongoPreview(cfg *config.Config, collection string, limit int) (string, int64, error) {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return "", 0, err
	}
	defer client.Disconnect(context.Background())

	coll := database.Collection(collection)
	filter := bson.D{}
	text, count, err := fetchMongoDocuments(coll, filter, limit)
	if err != nil {
		return "", 0, err
	}
	return text, count, nil
}

func runMongoCollectionCommand(cfg *config.Config, collection, command string, limit int) (string, string, bool, error) {
	client, database, err := connectMongoDatabase(cfg)
	if err != nil {
		return "", "", false, err
	}
	defer client.Disconnect(context.Background())

	coll := database.Collection(collection)
	action, payload := splitMongoCommand(command)
	if action == "" {
		return "", "", false, fmt.Errorf("command is required")
	}

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
		text, count, err := fetchMongoDocuments(coll, filter, limit)
		if err != nil {
			return "", "", false, err
		}
		message := fmt.Sprintf("%d document(s) matched.", count)
		return text, message, false, nil
	default:
		return "", "", false, fmt.Errorf("unsupported command: %s", action)
	}
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

func fetchMongoDocuments(coll *mongo.Collection, filter interface{}, limit int) (string, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	opts := options.Find()
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return "", 0, err
	}
	defer cursor.Close(ctx)

	var docs []map[string]interface{}
	for cursor.Next(ctx) {
		var doc map[string]interface{}
		if err := cursor.Decode(&doc); err != nil {
			return "", 0, err
		}
		docs = append(docs, doc)
	}
	if err := cursor.Err(); err != nil {
		return "", 0, err
	}

	text, err := formatMongoDocuments(docs)
	if err != nil {
		return "", 0, err
	}

	var count int64
	if isEmptyFilter(filter) {
		count, err = coll.EstimatedDocumentCount(ctx)
	} else {
		count, err = coll.CountDocuments(ctx, filter)
	}
	if err != nil {
		return "", 0, err
	}

	return text, count, nil
}

func formatMongoDocuments(docs []map[string]interface{}) (string, error) {
	if len(docs) == 0 {
		return "No documents found.", nil
	}
	var builder strings.Builder
	for i, doc := range docs {
		payload, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return "", err
		}
		builder.Write(payload)
		if i < len(docs)-1 {
			builder.WriteString("\n---\n")
		}
	}
	return builder.String(), nil
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

func isEmptyFilter(filter interface{}) bool {
	switch f := filter.(type) {
	case bson.M:
		return len(f) == 0
	case bson.D:
		return len(f) == 0
	default:
		return filter == nil
	}
}
