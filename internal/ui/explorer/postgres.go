package explorer

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/kadirbelkuyu/DBRTS/internal/config"

	_ "github.com/lib/pq"
)

type pgTable struct {
	Schema string
	Name   string
}

func runPostgresExplorer(cfg *config.Config) error {
	conn := cfg.GetConnectionString()
	if strings.TrimSpace(conn) == "" {
		return fmt.Errorf("postgres config is incomplete")
	}

	fmt.Println("Connecting to PostgreSQL...")
	fmt.Printf("Host: %s:%d\n", cfg.Database.Host, cfg.Database.Port)
	fmt.Printf("Database: %s\n\n", cfg.Database.Database)

	db, err := sql.Open("postgres", conn)
	if err != nil {
		return fmt.Errorf("failed to open database connection: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.PingContext(ctx); err != nil {
		cancel()
		return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}
	cancel()

	fmt.Println("Connected successfully! Loading explorer...")

	var tables []pgTable

	app := tview.NewApplication()
	list := tview.NewList().ShowSecondaryText(false)
	dataTable := tview.NewTable().SetFixed(1, 0).SetSelectable(true, false)
	meta := tview.NewTextView().SetDynamicColors(true)
	pages := tview.NewPages()

	list.AddItem("Loading tables…", "", 0, nil)
	meta.SetText("Connecting to PostgreSQL…")

	refresh := func(index int) {
		if index < 0 || index >= len(tables) {
			return
		}
		table := tables[index]
		go renderPostgresTable(app, db, table, dataTable, meta)
	}

	list.SetChangedFunc(func(index int, main, secondary string, shortcut rune) {
		refresh(index)
	})
	list.SetSelectedFunc(func(index int, main, secondary string, shortcut rune) {
		refresh(index)
	})

	var loadOnce sync.Once
	startLoader := func() {
		go func() {
			loaded, err := loadPostgresTables(db)
			if err != nil {
				queueUpdate(app, func() {
					list.Clear()
					list.AddItem("Failed to load tables", "", 0, nil)
					meta.SetText(fmt.Sprintf("[red]%v", err))
				})
				return
			}

			if len(loaded) == 0 {
				queueUpdate(app, func() {
					list.Clear()
					list.AddItem("No tables found", "", 0, nil)
					meta.SetText(fmt.Sprintf("No tables detected in %s", cfg.Database.Database))
				})
				return
			}

			queueUpdate(app, func() {
				tables = loaded
				list.Clear()
				for _, tbl := range tables {
					table := tbl
					list.AddItem(table.Schema+"."+table.Name, "", 0, func() {
						go renderPostgresTable(app, db, table, dataTable, meta)
					})
				}
				list.SetCurrentItem(0)
				go renderPostgresTable(app, db, tables[0], dataTable, meta)
				meta.SetText("[::b]Select a table to inspect.[-:-:-]\nPress ':' to run SQL, 'r' to refresh preview, 'q' to exit.")
			})
		}()
	}

	app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		loadOnce.Do(startLoader)
		return false
	})

	layout := tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(list.SetBorder(true).SetTitle("Tables"), 30, 1, true).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(dataTable.SetBorder(true).SetTitle("Preview"), 0, 3, false).
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
					if index := list.GetCurrentItem(); index >= 0 && index < len(tables) {
						table := tables[index]
						go renderPostgresTable(app, db, table, dataTable, meta)
					}
					return nil
				case ':':
					showPostgresCommandModal(app, pages, list, db, dataTable, meta)
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

func loadPostgresTables(db *sql.DB) ([]pgTable, error) {
	rows, err := db.Query(`
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

func renderPostgresTable(app *tview.Application, db *sql.DB, table pgTable, view *tview.Table, meta *tview.TextView) {
	queueUpdate(app, func() {
		meta.SetText(fmt.Sprintf("Loading %s.%s …", table.Schema, table.Name))
		view.Clear()
	})

	rows, columns, count, err := fetchPostgresSnapshot(db, table)
	if err != nil {
		queueUpdate(app, func() {
			view.Clear()
			meta.SetText(fmt.Sprintf("[red]%v", err))
		})
		return
	}

	queueUpdate(app, func() {
		view.Clear()
		for i, col := range columns {
			cell := tview.NewTableCell(col).SetSelectable(false).SetAlign(tview.AlignCenter).SetAttributes(tcell.AttrBold)
			view.SetCell(0, i, cell)
		}
		for r, row := range rows {
			for c, val := range row {
				view.SetCell(r+1, c, tview.NewTableCell(val).SetExpansion(1))
			}
		}

		text := fmt.Sprintf("[::b]%s.%s[-:-:-]\nRows: %s\nPreview size: %d\n':' to run SQL • 'r' to refresh • 'q' to exit",
			table.Schema,
			table.Name,
			count,
			len(rows),
		)
		meta.SetText(text)
	})
}

func fetchPostgresSnapshot(db *sql.DB, table pgTable) ([][]string, []string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := fmt.Sprintf("SELECT * FROM %s.%s LIMIT 200", quoteIdent(table.Schema), quoteIdent(table.Name))
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, nil, "", err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, "", err
	}

	var data [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, nil, "", err
		}
		row := make([]string, len(columns))
		for i, v := range values {
			row[i] = formatCell(v)
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, "", err
	}

	var count sql.NullInt64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", quoteIdent(table.Schema), quoteIdent(table.Name))
	if err := db.QueryRowContext(ctx, countQuery).Scan(&count); err != nil {
		return nil, nil, "", err
	}

	return data, columns, strconv.FormatInt(count.Int64, 10), nil
}

func quoteIdent(input string) string {
	return `"` + strings.ReplaceAll(input, `"`, `""`) + `"`
}

func formatCell(value interface{}) string {
	if value == nil {
		return "NULL"
	}
	switch v := value.(type) {
	case []byte:
		return string(v)
	case time.Time:
		return v.Format(time.RFC3339)
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func showPostgresCommandModal(app *tview.Application, pages *tview.Pages, list *tview.List, db *sql.DB, view *tview.Table, meta *tview.TextView) {
	const modalName = "postgres-sql"

	input := tview.NewInputField().
		SetLabel("SQL> ").
		SetFieldWidth(80)

	info := tview.NewTextView().
		SetDynamicColors(true).
		SetText("SELECT queries render inside the preview (max 200 rows).\nINSERT/UPDATE/DELETE statements execute immediately against this database.")

	form := tview.NewForm().
		AddFormItem(input).
		AddButton("Run", func() {
			sqlText := strings.TrimSpace(input.GetText())
			pages.RemovePage(modalName)
			app.SetFocus(list)
			if sqlText == "" {
				return
			}
			go executePostgresCommand(app, db, sqlText, view, meta)
		}).
		AddButton("Cancel", func() {
			pages.RemovePage(modalName)
			app.SetFocus(list)
		})

	form.SetBorder(true).SetTitle("Execute SQL")

	wrapper := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(info, 3, 1, false).
		AddItem(form, 0, 2, true)

	pages.AddPage(modalName, newModal(wrapper, 100, 12), true, true)
	app.SetFocus(input)
}

func executePostgresCommand(app *tview.Application, db *sql.DB, sqlText string, view *tview.Table, meta *tview.TextView) {
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return
	}

	if isSelectStatement(sqlText) {
		rows, columns, err := runPostgresSelectQuery(db, sqlText, 200)
		if err != nil {
			queueUpdate(app, func() {
				meta.SetText(fmt.Sprintf("[red]SQL error: %v", err))
			})
			return
		}

		queueUpdate(app, func() {
			view.Clear()
			for i, col := range columns {
				cell := tview.NewTableCell(col).SetSelectable(false).SetAlign(tview.AlignCenter).SetAttributes(tcell.AttrBold)
				view.SetCell(0, i, cell)
			}
			for r, row := range rows {
				for c, val := range row {
					view.SetCell(r+1, c, tview.NewTableCell(val).SetExpansion(1))
				}
			}
			meta.SetText(fmt.Sprintf("[::b]Query result[-:-:-]\nRows returned: %d\nPreview limited to 200 rows.", len(rows)))
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := db.ExecContext(ctx, sqlText)
	if err != nil {
		queueUpdate(app, func() {
			meta.SetText(fmt.Sprintf("[red]SQL error: %v", err))
		})
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		rowsAffected = 0
	}

	queueUpdate(app, func() {
		meta.SetText(fmt.Sprintf("[green]Statement executed.[-:-:-]\nRows affected: %d", rowsAffected))
	})

}

func isSelectStatement(sqlText string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(sqlText))
	return strings.HasPrefix(trimmed, "select") || strings.HasPrefix(trimmed, "with")
}

func runPostgresSelectQuery(db *sql.DB, query string, limit int) ([][]string, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
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
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(columns))
		for i, v := range values {
			row[i] = formatCell(v)
		}
		data = append(data, row)
		if limit > 0 && len(data) >= limit {
			break
		}
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return data, columns, nil
}
