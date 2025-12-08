package explorer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
)

func runMongoExplorer(cfg *config.Config) error {
	uri := cfg.GetMongoURI()
	if strings.TrimSpace(uri) == "" {
		return fmt.Errorf("mongodb config is incomplete")
	}
	if strings.TrimSpace(cfg.Database.Database) == "" {
		return fmt.Errorf("database field is required for MongoDB explorer")
	}

	fmt.Println("Connecting to MongoDB...")
	fmt.Printf("URI: %s\n", maskURI(uri))
	fmt.Printf("Database: %s\n\n", cfg.Database.Database)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	defer client.Disconnect(context.Background())

	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	fmt.Println("Connected successfully! Loading explorer...")

	database := client.Database(cfg.Database.Database)
	var names []string

	app := tview.NewApplication()
	list := tview.NewList().ShowSecondaryText(false)
	docView := tview.NewTextView().SetDynamicColors(true).SetWrap(true)
	meta := tview.NewTextView().SetDynamicColors(true)
	pages := tview.NewPages()

	list.AddItem("Loading collections…", "", 0, nil)
	meta.SetText("Connecting to MongoDB…")

	refresh := func(index int) {
		if index < 0 || index >= len(names) {
			return
		}
		go renderMongoCollection(app, database.Collection(names[index]), docView, meta)
	}

	list.SetChangedFunc(func(index int, main, secondary string, shortcut rune) {
		refresh(index)
	})

	var loadOnce sync.Once
	startLoader := func() {
		go func() {
			namesCtx, cancelNames := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelNames()

			loaded, err := database.ListCollectionNames(namesCtx, bson.D{})
			if err != nil {
				queueUpdate(app, func() {
					list.Clear()
					list.AddItem("Failed to load collections", "", 0, nil)
					meta.SetText(fmt.Sprintf("[red]%v", err))
				})
				return
			}

			if len(loaded) == 0 {
				queueUpdate(app, func() {
					list.Clear()
					list.AddItem("No collections found", "", 0, nil)
					meta.SetText(fmt.Sprintf("No collections detected in %s", cfg.Database.Database))
				})
				return
			}

			queueUpdate(app, func() {
				names = loaded
				list.Clear()
				for _, name := range names {
					collection := name
					list.AddItem(collection, "", 0, func() {
						go renderMongoCollection(app, database.Collection(collection), docView, meta)
					})
				}
				list.SetCurrentItem(0)
				go renderMongoCollection(app, database.Collection(names[0]), docView, meta)
				meta.SetText("[::b]Select a collection to inspect.[-:-:-]\n':' to insert/update/delete • 'r' to refresh • 'q' to exit.")
			})
		}()
	}

	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		loadOnce.Do(startLoader)
		return false
	})

	layout := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(list.SetBorder(true).SetTitle("Collections"), 30, 1, true).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(docView.SetBorder(true).SetTitle("Documents"), 0, 3, false).
			AddItem(meta.SetBorder(true).SetTitle("Details"), 7, 1, false),
			0, 3, false)

	pages.AddPage("main", layout, true, true)

	app.SetRoot(pages, true).
		SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyRune {
				switch event.Rune() {
				case 'q', 'Q':
					app.Stop()
					return nil
				case 'r', 'R':
					if index := list.GetCurrentItem(); index >= 0 && index < len(names) {
						go renderMongoCollection(app, database.Collection(names[index]), docView, meta)
					}
					return nil
				case ':':
					showMongoCommandModal(app, pages, list, database, names, docView, meta)
					return nil
				}
			}
			return event
		})

	fmt.Println("Starting TUI... (Press 'q' to exit)")
	if err := app.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

func renderMongoCollection(app *tview.Application, coll *mongo.Collection, docView, meta *tview.TextView) {
	queueUpdate(app, func() {
		docView.SetText("Loading…")
		meta.SetText(fmt.Sprintf("Loading %s …", coll.Name()))
	})

	text, count, err := fetchMongoDocuments(coll, bson.D{}, 50)
	if err != nil {
		queueUpdate(app, func() {
			docView.SetText("")
			meta.SetText(fmt.Sprintf("[red]%v", err))
		})
		return
	}

	queueUpdate(app, func() {
		docView.SetText(text)
		info := fmt.Sprintf("[::b]%s[-:-:-]\nDocuments: %d\nPreview limit: 50\n':' to edit • 'r' to refresh • 'q' to exit",
			coll.Name(),
			count,
		)
		meta.SetText(info)
	})
}

func showMongoCommandModal(app *tview.Application, pages *tview.Pages, list *tview.List, database *mongo.Database, names []string, docView, meta *tview.TextView) {
	index := list.GetCurrentItem()
	if index < 0 || index >= len(names) {
		return
	}

	const modalName = "mongo-command"
	collectionName := names[index]

	input := tview.NewInputField().
		SetLabel("Command> ").
		SetFieldWidth(80)

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetText("Supported commands:\n insert {\"field\":\"value\"}\n update {\"filter\":{},\"update\":{\"$set\":{}}}\n delete {\"filter\":{}}\n find {\"status\":\"open\"}")

	form := tview.NewForm().
		AddFormItem(input).
		AddButton("Run", func() {
			command := strings.TrimSpace(input.GetText())
			pages.RemovePage(modalName)
			app.SetFocus(list)
			if command == "" {
				return
			}
			coll := database.Collection(collectionName)
			go executeMongoCommand(app, coll, command, docView, meta)
		}).
		AddButton("Cancel", func() {
			pages.RemovePage(modalName)
			app.SetFocus(list)
		})

	form.SetBorder(true).SetTitle("MongoDB Command")

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(info, 5, 1, false).
		AddItem(form, 0, 2, true)

	pages.AddPage(modalName, newModal(wrapper, 110, 14), true, true)
	app.SetFocus(input)
}

func executeMongoCommand(app *tview.Application, coll *mongo.Collection, command string, docView, meta *tview.TextView) {
	action, payload := splitMongoCommand(command)
	if action == "" {
		queueUpdate(app, func() {
			meta.SetText("[red]Command is required.")
		})
		return
	}

	switch action {
	case "insert":
		doc, err := decodeMongoDocument(payload)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Invalid insert payload: %v", err))
			})
			return
		}
		if len(doc) == 0 {
			queueUpdate(app, func() {
				meta.SetText("[red]Insert payload cannot be empty.")
			})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := coll.InsertOne(ctx, doc)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Insert failed: %v", err))
			})
			return
		}
		queueUpdate(app, func() {
			meta.SetText(fmt.Sprintf("[green]Insert successful.[-:-:-]\nInserted ID: %v\nPress 'r' to refresh preview.", result.InsertedID))
		})
	case "update":
		filter, update, err := parseMongoUpdatePayload(payload)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Invalid update payload: %v", err))
			})
			return
		}
		if len(filter) == 0 {
			queueUpdate(app, func() {
				meta.SetText("[red]Update filter is required.")
			})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := coll.UpdateMany(ctx, filter, update)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Update failed: %v", err))
			})
			return
		}
		queueUpdate(app, func() {
			meta.SetText(fmt.Sprintf("[green]Update successful.[-:-:-]\nMatched: %d • Modified: %d\nPress 'r' to refresh preview.", result.MatchedCount, result.ModifiedCount))
		})
	case "delete":
		filter, err := decodeMongoDocument(payload)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Invalid delete payload: %v", err))
			})
			return
		}
		if len(filter) == 0 {
			queueUpdate(app, func() {
				meta.SetText("[red]Delete filter is required.")
			})
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result, err := coll.DeleteMany(ctx, filter)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Delete failed: %v", err))
			})
			return
		}
		queueUpdate(app, func() {
			meta.SetText(fmt.Sprintf("[green]Delete successful.[-:-:-]\nRemoved: %d\nPress 'r' to refresh preview.", result.DeletedCount))
		})
	case "find":
		filter, err := decodeMongoDocument(payload)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Invalid find payload: %v", err))
			})
			return
		}
		text, count, err := fetchMongoDocuments(coll, filter, 50)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]Find failed: %v", err))
			})
			return
		}
		queueUpdate(app, func() {
			docView.SetText(text)
			meta.SetText(fmt.Sprintf("[::b]%s[-:-:-]\nDocuments matched: %d\nPreview limit: 50", coll.Name(), count))
		})
	default:
		queueUpdate(app, func() {
			meta.SetText("[red]Unsupported command. Use insert, update, delete, or find.")
		})
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

func fetchMongoDocuments(coll *mongo.Collection, filter interface{}, limit int) (string, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if filter == nil {
		filter = bson.D{}
	}

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

func maskURI(uri string) string {
	if strings.Contains(uri, "@") {
		parts := strings.SplitN(uri, "@", 2)
		if len(parts) == 2 {
			prefix := parts[0]
			if strings.Contains(prefix, "://") {
				schemeParts := strings.SplitN(prefix, "://", 2)
				if len(schemeParts) == 2 {
					return schemeParts[0] + "://***:***@" + parts[1]
				}
			}
		}
	}
	return uri
}
