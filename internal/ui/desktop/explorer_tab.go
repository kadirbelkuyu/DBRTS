package desktop

import (
	"encoding/json"
	"fmt"
	"strings"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/profiles"
)

type explorerItem struct {
	Title    string
	Subtitle string
	payload  interface{}
}

type explorerView struct {
	app *App

	content fyne.CanvasObject

	profilePicker *widget.Select
	profileLookup map[string]profiles.Profile

	filterEntry *widget.Entry
	limitEntry  *widget.Entry

	list  *widget.List
	items []explorerItem

	previewTable *widget.Table
	mongoDocList *widget.List
	previewStack *fyne.Container

	metaLabel   *widget.Label
	detailJSON  *widget.Entry
	statusLabel *widget.Label

	commandEntry *widget.Entry
	runButton    *widget.Button

	addButton       *widget.Button
	duplicateButton *widget.Button
	editButton      *widget.Button
	deleteButton    *widget.Button

	activeProfile *profiles.Profile
	activeConfig  *config.Config
	dbType        string

	currentTable      *pgTable
	currentCollection string

	pgPreview    *pgPreview
	pgColumns    []pgColumn
	selectedRow  int
	mongoPreview *mongoPreview
	summaries    []string
	selectedDoc  int
}

func newExplorerView(app *App) *explorerView {
	view := &explorerView{
		app:           app,
		profileLookup: make(map[string]profiles.Profile),
		selectedRow:   -1,
		selectedDoc:   -1,
	}

	view.profilePicker = widget.NewSelect([]string{}, func(value string) {
		view.onProfileSelected(value)
	})
	view.profilePicker.PlaceHolder = "Select profile…"

	view.filterEntry = widget.NewEntry()
	view.filterEntry.SetPlaceHolder("status = 'active'")
	view.filterEntry.OnSubmitted = func(string) {
		view.reloadSelection()
	}

	view.limitEntry = widget.NewEntry()
	view.limitEntry.SetText("200")
	view.limitEntry.OnSubmitted = func(string) {
		view.reloadSelection()
	}

	view.list = widget.NewList(
		func() int { return len(view.items) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("")
			title.TextStyle = fyne.TextStyle{Bold: true}
			subtitle := widget.NewLabel("")
			subtitle.TextStyle = fyne.TextStyle{Italic: true}
			return container.NewVBox(title, subtitle)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			index := int(id)
			if index < 0 || index >= len(view.items) {
				return
			}
			entry := view.items[index]
			box := item.(*fyne.Container)
			if len(box.Objects) >= 2 {
				box.Objects[0].(*widget.Label).SetText(entry.Title)
				box.Objects[1].(*widget.Label).SetText(entry.Subtitle)
			}
		},
	)
	view.list.OnSelected = func(id widget.ListItemID) {
		view.onItemSelected(int(id))
	}

	view.previewTable = widget.NewTable(
		view.tableSize,
		func() fyne.CanvasObject { return widget.NewLabel("") },
		view.updateTableCell,
	)
	view.previewTable.OnSelected = func(id widget.TableCellID) {
		if id.Row == 0 {
			return
		}
		view.onRowSelected(id.Row - 1)
	}

	view.mongoDocList = widget.NewList(
		func() int { return len(view.summaries) },
		func() fyne.CanvasObject {
			return widget.NewLabel("Document")
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			index := int(id)
			if index < 0 || index >= len(view.summaries) {
				return
			}
			item.(*widget.Label).SetText(view.summaries[index])
		},
	)
	view.mongoDocList.OnSelected = func(id widget.ListItemID) {
		view.onDocumentSelected(int(id))
	}

	tableContainer := container.NewBorder(nil, nil, nil, nil, view.previewTable)
	mongoContainer := container.NewBorder(nil, nil, nil, nil, view.mongoDocList)
	view.previewStack = container.NewStack(tableContainer, mongoContainer)

	view.metaLabel = widget.NewLabel("Pick a profile to inspect its tables or collections.")
	view.metaLabel.Wrapping = fyne.TextWrapWord

	view.detailJSON = widget.NewMultiLineEntry()
	view.detailJSON.SetMinRowsVisible(6)
	view.detailJSON.Disable()

	detailCard := widget.NewCard("Details", "", container.NewBorder(view.metaLabel, nil, nil, nil, view.detailJSON))

	view.statusLabel = widget.NewLabel("")
	view.statusLabel.TextStyle = fyne.TextStyle{Italic: true}

	view.commandEntry = widget.NewMultiLineEntry()
	view.commandEntry.SetMinRowsVisible(4)
	view.commandEntry.Wrapping = fyne.TextWrapWord
	view.commandEntry.PlaceHolder = "Postgres: SELECT * FROM public.users LIMIT 20\nMongo: insert {\"name\":\"alpha\"}"

	view.runButton = widget.NewButtonWithIcon("Run", theme.MediaPlayIcon(), func() {
		view.runCommand()
	})

	view.addButton = widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		view.createRecord(false)
	})
	view.addButton.Disable()

	view.duplicateButton = widget.NewButtonWithIcon("Duplicate", theme.ContentCopyIcon(), func() {
		view.createRecord(true)
	})
	view.duplicateButton.Disable()

	view.editButton = widget.NewButtonWithIcon("Edit", theme.DocumentCreateIcon(), func() {
		view.editSelection()
	})
	view.editButton.Disable()

	view.deleteButton = widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		view.deleteSelection()
	})
	view.deleteButton.Disable()

	connectBtn := widget.NewButtonWithIcon("Connect", theme.ConfirmIcon(), func() {
		view.connectSelectedProfile()
	})
	reloadBtn := widget.NewButtonWithIcon("Reload", theme.ViewRefreshIcon(), func() {
		view.reloadSelection()
	})
	clearBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		view.clearState()
	})

	filterControls := container.NewGridWithColumns(3,
		container.NewVBox(widget.NewLabel("Filter / WHERE"), view.filterEntry),
		container.NewVBox(widget.NewLabel("Limit"), view.limitEntry),
		container.NewVBox(widget.NewLabel("Apply"), widget.NewButtonWithIcon("Apply", theme.ViewRefreshIcon(), func() {
			view.reloadSelection()
		})),
	)

	dataActions := container.NewGridWithColumns(4, view.addButton, view.duplicateButton, view.editButton, view.deleteButton)

	controls := container.NewVBox(
		container.NewGridWithColumns(4,
			container.NewVBox(widget.NewLabel("Profile"), view.profilePicker),
			connectBtn,
			reloadBtn,
			clearBtn,
		),
		filterControls,
		dataActions,
	)

	previewCard := widget.NewCard("Preview", "", view.previewStack)

	commandHelper := widget.NewLabel("Command palette • Postgres accepts SQL, Mongo uses verb + JSON payload (insert/update/delete/find).")
	commandHelper.Wrapping = fyne.TextWrapWord
	commandArea := container.NewBorder(nil, nil, nil, view.runButton, view.commandEntry)

	footer := container.NewVBox(
		widget.NewSeparator(),
		commandHelper,
		commandArea,
		view.statusLabel,
	)

	right := container.NewBorder(nil, detailCard, nil, nil, previewCard)
	split := container.NewHSplit(widget.NewCard("Collections / Tables", "", container.NewMax(view.list)), right)
	split.SetOffset(0.33)

	view.content = container.NewBorder(controls, footer, nil, nil, split)
	view.showMongo(false)

	return view
}

func (a *App) buildExplorerTab() fyne.CanvasObject {
	if a.explorer == nil {
		a.explorer = newExplorerView(a)
	}
	return a.explorer.canvas()
}

func (v *explorerView) canvas() fyne.CanvasObject {
	return v.content
}

func (v *explorerView) updateProfiles(list []profiles.Profile) {
	v.profileLookup = make(map[string]profiles.Profile)
	options := make([]string, len(list))
	var selected string
	for i, profile := range list {
		label := fmt.Sprintf("%s (%s)", profile.Name, strings.ToUpper(profile.Type))
		options[i] = label
		v.profileLookup[label] = profile
		if v.activeProfile != nil && profile.Path == v.activeProfile.Path {
			selected = label
		}
	}
	v.profilePicker.Options = options
	v.profilePicker.Refresh()
	if selected != "" {
		v.profilePicker.SetSelected(selected)
	} else if len(options) == 0 {
		v.profilePicker.ClearSelected()
		v.clearState()
	}
}

func (v *explorerView) onProfileSelected(label string) {
	profile, ok := v.profileLookup[label]
	if !ok {
		return
	}
	cfg, err := config.LoadConfig(profile.Path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("load config: %w", err), v.app.window)
		v.setStatus("Cannot open %s: %v", profile.Name, err)
		return
	}
	v.activeProfile = &profile
	v.activeConfig = cfg
	v.dbType = cfg.Database.Type
	v.adjustPlaceholders()
	v.setStatus("Connected to %s (%s).", profile.Name, strings.ToUpper(cfg.Database.Type))
	v.loadInventory()
}

func (v *explorerView) adjustPlaceholders() {
	switch strings.ToLower(v.dbType) {
	case "postgres":
		v.filterEntry.SetPlaceHolder("status = 'active'")
		v.limitEntry.SetText("200")
	case "mongo":
		v.filterEntry.SetPlaceHolder("{\"field\":\"value\"}")
		v.limitEntry.SetText("50")
	default:
		v.filterEntry.SetPlaceHolder("")
	}
}

func (v *explorerView) connectSelectedProfile() {
	if v.profilePicker.Selected == "" {
		dialog.ShowInformation("Select profile", "Pick a saved profile first.", v.app.window)
		return
	}
	v.onProfileSelected(v.profilePicker.Selected)
}

func (v *explorerView) loadInventory() {
	if v.activeConfig == nil {
		return
	}
	v.items = nil
	v.list.Refresh()
	v.metaLabel.SetText("Loading schema information…")
	v.showMongo(false)

	switch strings.ToLower(v.dbType) {
	case "postgres":
		go func(cfg *config.Config) {
			tables, err := listPostgresTables(cfg)
			v.app.runOnUI(func() {
				if err != nil {
					v.metaLabel.SetText(fmt.Sprintf("Failed to load tables: %v", err))
					v.setStatus("Table load failed: %v", err)
					return
				}
				v.items = make([]explorerItem, len(tables))
				for i, table := range tables {
					t := table
					v.items[i] = explorerItem{
						Title:    fmt.Sprintf("%s.%s", table.Schema, table.Name),
						Subtitle: "Table",
						payload:  &t,
					}
				}
				v.list.Refresh()
				if len(v.items) > 0 {
					v.list.Select(0)
					v.onItemSelected(0)
				} else {
					v.metaLabel.SetText("No tables found.")
				}
			})
		}(v.activeConfig)
	case "mongo":
		go func(cfg *config.Config) {
			names, err := listMongoCollections(cfg)
			v.app.runOnUI(func() {
				if err != nil {
					v.metaLabel.SetText(fmt.Sprintf("Failed to load collections: %v", err))
					v.setStatus("Collection load failed: %v", err)
					return
				}
				v.items = make([]explorerItem, len(names))
				for i, name := range names {
					v.items[i] = explorerItem{Title: name, Subtitle: "Collection", payload: name}
				}
				v.list.Refresh()
				if len(v.items) > 0 {
					v.list.Select(0)
					v.onItemSelected(0)
				} else {
					v.metaLabel.SetText("No collections found.")
				}
			})
		}(v.activeConfig)
	default:
		v.metaLabel.SetText(fmt.Sprintf("Explorer does not support %s yet.", v.dbType))
	}
}

func (v *explorerView) onItemSelected(index int) {
	if index < 0 || index >= len(v.items) {
		return
	}
	item := v.items[index]
	switch payload := item.payload.(type) {
	case *pgTable:
		v.currentTable = payload
		v.currentCollection = ""
		v.loadPostgresPreview()
	case string:
		v.currentCollection = payload
		v.currentTable = nil
		v.loadMongoPreview()
	}
}

func (v *explorerView) reloadSelection() {
	if v.currentTable != nil {
		v.loadPostgresPreview()
	} else if v.currentCollection != "" {
		v.loadMongoPreview()
	}
}

func (v *explorerView) loadPostgresPreview() {
	if v.activeConfig == nil || v.currentTable == nil {
		return
	}
	filter := strings.TrimSpace(v.filterEntry.Text)
	limit := parseLimitInput(v.limitEntry.Text, 200)
	table := *v.currentTable
	v.metaLabel.SetText(fmt.Sprintf("Loading %s.%s …", table.Schema, table.Name))
	v.showMongo(false)
	v.addButton.Enable()
	v.duplicateButton.Disable()
	v.editButton.Disable()
	v.deleteButton.Disable()

	go func(cfg *config.Config) {
		preview, err := fetchPostgresPreview(cfg, table, filter, limit)
		columns, colErr := loadPostgresColumns(cfg, table)
		v.app.runOnUI(func() {
			if err != nil {
				v.metaLabel.SetText(fmt.Sprintf("Preview failed: %v", err))
				v.setStatus("Preview failed: %v", err)
				return
			}
			if colErr != nil {
				v.setStatus("Column metadata: %v", colErr)
			}
			v.pgPreview = preview
			v.pgColumns = columns
			v.selectedRow = -1
			v.previewTable.Refresh()
			v.detailJSON.Enable()
			v.detailJSON.SetText("")
			v.detailJSON.Disable()
			v.metaLabel.SetText(fmt.Sprintf("%s.%s • %d column(s) • %d row(s) previewed", table.Schema, table.Name, len(preview.Columns), len(preview.Rows)))
			v.setStatus("Preview refreshed.")
		})
	}(v.activeConfig)
}

func (v *explorerView) loadMongoPreview() {
	if v.activeConfig == nil || v.currentCollection == "" {
		return
	}
	filter := strings.TrimSpace(v.filterEntry.Text)
	limit := parseLimitInput(v.limitEntry.Text, 50)
	collection := v.currentCollection
	v.metaLabel.SetText(fmt.Sprintf("Loading %s …", collection))
	v.showMongo(true)
	v.addButton.Enable()
	v.duplicateButton.Disable()
	v.editButton.Disable()
	v.deleteButton.Disable()

	go func(cfg *config.Config) {
		preview, err := fetchMongoPreview(cfg, collection, filter, limit)
		v.app.runOnUI(func() {
			if err != nil {
				v.metaLabel.SetText(fmt.Sprintf("Preview failed: %v", err))
				v.setStatus("Preview failed: %v", err)
				return
			}
			v.mongoPreview = preview
			v.summaries = make([]string, len(preview.Documents))
			for i, doc := range preview.Documents {
				v.summaries[i] = summarizeDocument(doc)
			}
			v.mongoDocList.Refresh()
			if len(v.summaries) > 0 {
				v.mongoDocList.Select(0)
				v.onDocumentSelected(0)
			} else {
				v.detailJSON.Enable()
				v.detailJSON.SetText("No documents found.")
				v.detailJSON.Disable()
			}
			v.metaLabel.SetText(fmt.Sprintf("%s • %d document(s) matched", collection, preview.Count))
			v.setStatus("Preview refreshed.")
		})
	}(v.activeConfig)
}

func (v *explorerView) tableSize() (int, int) {
	if v.pgPreview == nil || len(v.pgPreview.Columns) == 0 {
		return 1, 1
	}
	return len(v.pgPreview.Rows) + 1, len(v.pgPreview.Columns)
}

func (v *explorerView) updateTableCell(id widget.TableCellID, cell fyne.CanvasObject) {
	label := cell.(*widget.Label)
	if v.pgPreview == nil || len(v.pgPreview.Columns) == 0 {
		label.SetText("")
		return
	}
	if id.Row == 0 {
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.SetText(v.pgPreview.Columns[id.Col])
		return
	}
	row := id.Row - 1
	if row < len(v.pgPreview.Rows) && id.Col < len(v.pgPreview.Columns) {
		label.TextStyle = fyne.TextStyle{}
		label.SetText(v.pgPreview.Rows[row][id.Col])
	} else {
		label.SetText("")
	}
}

func (v *explorerView) onRowSelected(row int) {
	if v.pgPreview == nil || row < 0 || row >= len(v.pgPreview.Rows) {
		return
	}
	v.selectedRow = row
	v.duplicateButton.Enable()
	v.editButton.Enable()
	v.deleteButton.Enable()
	rowMap := v.pgPreview.Raw[row]
	payload := make(map[string]interface{})
	for _, col := range v.pgPreview.Columns {
		payload[col] = rowMap[col]
	}
	jsonText, _ := json.MarshalIndent(payload, "", "  ")
	v.detailJSON.Enable()
	v.detailJSON.SetText(string(jsonText))
	v.detailJSON.Disable()
}

func (v *explorerView) onDocumentSelected(index int) {
	if v.mongoPreview == nil || index < 0 || index >= len(v.mongoPreview.Documents) {
		return
	}
	v.selectedDoc = index
	v.duplicateButton.Enable()
	v.editButton.Enable()
	v.deleteButton.Enable()
	doc := v.mongoPreview.Documents[index]
	v.detailJSON.Enable()
	v.detailJSON.SetText(formatJSON(doc))
	v.detailJSON.Disable()
}

func (v *explorerView) showMongo(enabled bool) {
	children := v.previewStack.Objects
	if len(children) < 2 {
		return
	}
	if enabled {
		children[0].Hide()
		children[1].Show()
	} else {
		children[1].Hide()
		children[0].Show()
	}
}

func (v *explorerView) createRecord(duplicate bool) {
	switch strings.ToLower(v.dbType) {
	case "postgres":
		v.addPostgresRow(duplicate)
	case "mongo":
		v.addMongoDocument(duplicate)
	default:
		dialog.ShowInformation("Unsupported", "This engine does not support inserts yet.", v.app.window)
	}
}

func (v *explorerView) editSelection() {
	switch strings.ToLower(v.dbType) {
	case "postgres":
		v.editPostgresRow()
	case "mongo":
		v.editMongoDocument()
	}
}

func (v *explorerView) deleteSelection() {
	switch strings.ToLower(v.dbType) {
	case "postgres":
		v.deletePostgresRow()
	case "mongo":
		v.deleteMongoDocument()
	}
}

func (v *explorerView) editPostgresRow() {
	if v.activeConfig == nil || v.currentTable == nil || v.pgPreview == nil || v.selectedRow < 0 {
		return
	}
	rowMap := v.pgPreview.Raw[v.selectedRow]
	ctid := v.pgPreview.CTIDs[v.selectedRow]

	type editor struct {
		column pgColumn
		entry  *widget.Entry
		null   *widget.Check
	}
	editors := make([]*editor, len(v.pgColumns))
	form := container.NewVBox()

	for i, column := range v.pgColumns {
		entry := widget.NewEntry()
		entry.SetText(rowMap[column.Name])
		nullCheck := widget.NewCheck("NULL", func(on bool) {
			if on {
				entry.Disable()
			} else {
				entry.Enable()
			}
		})
		if rowMap[column.Name] == "" {
			entry.SetPlaceHolder("(empty)")
		}
		editors[i] = &editor{column: column, entry: entry, null: nullCheck}
		form.Add(container.NewVBox(widget.NewLabel(column.Name), entry, nullCheck))
	}

	dialog.ShowCustomConfirm("Edit Row", "Save", "Cancel", container.NewVScroll(form), func(ok bool) {
		if !ok {
			return
		}
		updates := make([]pgFieldInput, 0, len(editors))
		for _, ed := range editors {
			if ed.null.Checked {
				updates = append(updates, pgFieldInput{Column: ed.column.Name, Value: nil})
				continue
			}
			value := ed.entry.Text
			copied := value
			updates = append(updates, pgFieldInput{Column: ed.column.Name, Value: &copied})
		}
		if err := updatePostgresRow(v.activeConfig, *v.currentTable, updates, ctid); err != nil {
			dialog.ShowError(err, v.app.window)
			v.setStatus("Update failed: %v", err)
			return
		}
		v.setStatus("Row updated successfully.")
		v.reloadSelection()
	}, v.app.window)
}

func (v *explorerView) deletePostgresRow() {
	if v.activeConfig == nil || v.currentTable == nil || v.pgPreview == nil || v.selectedRow < 0 {
		return
	}
	ctid := v.pgPreview.CTIDs[v.selectedRow]
	dialog.ShowConfirm("Delete Row", "Are you sure you want to delete this row?", func(ok bool) {
		if !ok {
			return
		}
		if err := deletePostgresRow(v.activeConfig, *v.currentTable, ctid); err != nil {
			dialog.ShowError(err, v.app.window)
			v.setStatus("Delete failed: %v", err)
			return
		}
		v.setStatus("Row deleted.")
		v.reloadSelection()
	}, v.app.window)
}

func (v *explorerView) editMongoDocument() {
	if v.activeConfig == nil || v.currentCollection == "" || v.mongoPreview == nil || v.selectedDoc < 0 {
		return
	}
	doc := v.mongoPreview.Documents[v.selectedDoc]
	originalID, ok := doc["_id"]
	if !ok {
		dialog.ShowError(fmt.Errorf("document missing _id"), v.app.window)
		return
	}

	entry := widget.NewMultiLineEntry()
	entry.SetMinRowsVisible(10)
	entry.SetText(formatJSON(doc))

	dialog.ShowCustomConfirm("Edit Document", "Save", "Cancel", container.NewVScroll(entry), func(ok bool) {
		if !ok {
			return
		}
		var updated bson.M
		if err := bson.UnmarshalExtJSON([]byte(entry.Text), true, &updated); err != nil {
			dialog.ShowError(err, v.app.window)
			return
		}
		updated["_id"] = originalID
		filter := bson.M{"_id": originalID}
		if err := replaceMongoDocument(v.activeConfig, v.currentCollection, filter, updated); err != nil {
			dialog.ShowError(err, v.app.window)
			v.setStatus("Update failed: %v", err)
			return
		}
		v.setStatus("Document updated.")
		v.reloadSelection()
	}, v.app.window)
}

func (v *explorerView) deleteMongoDocument() {
	if v.activeConfig == nil || v.currentCollection == "" || v.mongoPreview == nil || v.selectedDoc < 0 {
		return
	}
	doc := v.mongoPreview.Documents[v.selectedDoc]
	originalID, ok := doc["_id"]
	if !ok {
		dialog.ShowError(fmt.Errorf("document missing _id"), v.app.window)
		return
	}
	dialog.ShowConfirm("Delete Document", "Delete the selected document?", func(ok bool) {
		if !ok {
			return
		}
		if err := deleteMongoDocument(v.activeConfig, v.currentCollection, bson.M{"_id": originalID}); err != nil {
			dialog.ShowError(err, v.app.window)
			v.setStatus("Delete failed: %v", err)
			return
		}
		v.setStatus("Document deleted.")
		v.reloadSelection()
	}, v.app.window)
}

func (v *explorerView) addPostgresRow(duplicate bool) {
	if v.activeConfig == nil || v.currentTable == nil || len(v.pgColumns) == 0 {
		dialog.ShowInformation("Unavailable", "Load a table before inserting rows.", v.app.window)
		return
	}
	var template map[string]string
	if duplicate {
		if v.pgPreview == nil || v.selectedRow < 0 {
			dialog.ShowInformation("Select row", "Choose a row to duplicate first.", v.app.window)
			return
		}
		template = v.pgPreview.Raw[v.selectedRow]
	}

	type editor struct {
		column       pgColumn
		entry        *widget.Entry
		nullCheck    *widget.Check
		defaultCheck *widget.Check
	}
	editors := make([]*editor, len(v.pgColumns))
	form := container.NewVBox()

	for i, column := range v.pgColumns {
		entry := widget.NewEntry()
		if template != nil {
			entry.SetText(template[column.Name])
		}
		nullCheck := widget.NewCheck("NULL", nil)
		defaultCheck := widget.NewCheck("DEFAULT", nil)
		nullCheck.OnChanged = func(on bool) {
			if on {
				entry.Disable()
			} else if !defaultCheck.Checked {
				entry.Enable()
			}
		}
		defaultCheck.OnChanged = func(on bool) {
			if on {
				entry.Disable()
				nullCheck.SetChecked(false)
				nullCheck.Disable()
			} else {
				nullCheck.Enable()
				if !nullCheck.Checked {
					entry.Enable()
				}
			}
		}

		editors[i] = &editor{column: column, entry: entry, nullCheck: nullCheck, defaultCheck: defaultCheck}
		form.Add(container.NewVBox(widget.NewLabel(column.Name), entry, container.NewGridWithColumns(2, nullCheck, defaultCheck)))
	}

	dialog.ShowCustomConfirm("Insert Row", "Insert", "Cancel", container.NewVScroll(form), func(ok bool) {
		if !ok {
			return
		}
		values := make([]pgFieldInput, 0, len(editors))
		for _, ed := range editors {
			switch {
			case ed.defaultCheck.Checked:
				values = append(values, pgFieldInput{Column: ed.column.Name, UseDefault: true})
			case ed.nullCheck.Checked:
				values = append(values, pgFieldInput{Column: ed.column.Name, Value: nil})
			default:
				text := ed.entry.Text
				val := text
				values = append(values, pgFieldInput{Column: ed.column.Name, Value: &val})
			}
		}
		if err := insertPostgresRow(v.activeConfig, *v.currentTable, values); err != nil {
			dialog.ShowError(err, v.app.window)
			v.setStatus("Insert failed: %v", err)
			return
		}
		v.setStatus("Row inserted.")
		v.reloadSelection()
	}, v.app.window)
}

func (v *explorerView) addMongoDocument(duplicate bool) {
	if v.activeConfig == nil || v.currentCollection == "" {
		dialog.ShowInformation("Select collection", "Choose a collection before inserting documents.", v.app.window)
		return
	}

	entry := widget.NewMultiLineEntry()
	entry.SetMinRowsVisible(10)
	if duplicate && v.mongoPreview != nil && v.selectedDoc >= 0 {
		doc := v.mongoPreview.Documents[v.selectedDoc]
		copyDoc := make(map[string]interface{}, len(doc))
		for k, val := range doc {
			if k == "_id" {
				continue
			}
			copyDoc[k] = val
		}
		entry.SetText(formatJSON(copyDoc))
	} else {
		entry.SetText("{\n  \n}")
	}

	dialog.ShowCustomConfirm("Insert Document", "Insert", "Cancel", container.NewVScroll(entry), func(ok bool) {
		if !ok {
			return
		}
		var doc bson.M
		if err := bson.UnmarshalExtJSON([]byte(entry.Text), true, &doc); err != nil {
			dialog.ShowError(err, v.app.window)
			return
		}
		if err := insertMongoDocument(v.activeConfig, v.currentCollection, doc); err != nil {
			dialog.ShowError(err, v.app.window)
			v.setStatus("Insert failed: %v", err)
			return
		}
		v.setStatus("Document inserted.")
		v.reloadSelection()
	}, v.app.window)
}

func (v *explorerView) runCommand() {
	command := strings.TrimSpace(v.commandEntry.Text)
	if command == "" {
		dialog.ShowInformation("Empty command", "Enter SQL or Mongo command first.", v.app.window)
		return
	}
	if v.activeConfig == nil {
		dialog.ShowInformation("No profile", "Connect to a profile first.", v.app.window)
		return
	}
	switch strings.ToLower(v.dbType) {
	case "postgres":
		v.runPostgresCommand(command)
	case "mongo":
		v.runMongoCommand(command)
	default:
		dialog.ShowInformation("Unsupported", fmt.Sprintf("%s explorer is not supported yet.", v.dbType), v.app.window)
	}
}

func (v *explorerView) runPostgresCommand(command string) {
	v.setStatus("Running SQL…")
	go func(cfg *config.Config, query string) {
		columns, rows, message, err := runPostgresStatement(cfg, query, 200)
		v.app.runOnUI(func() {
			if err != nil {
				dialog.ShowError(err, v.app.window)
				v.setStatus("SQL failed: %v", err)
				return
			}
			if len(columns) > 0 {
				v.pgPreview = &pgPreview{Columns: columns, Rows: rows}
				v.previewTable.Refresh()
				v.showMongo(false)
			}
			v.metaLabel.SetText(message)
			v.detailJSON.Enable()
			v.detailJSON.SetText("")
			v.detailJSON.Disable()
			v.setStatus(message)
		})
	}(v.activeConfig, command)
}

func (v *explorerView) runMongoCommand(command string) {
	if v.currentCollection == "" {
		dialog.ShowInformation("Select collection", "Pick a collection on the left to target the command.", v.app.window)
		return
	}
	v.setStatus("Running Mongo command…")
	go func(cfg *config.Config, coll, cmd string) {
		output, message, refresh, err := runMongoCollectionCommand(cfg, coll, cmd, 50)
		v.app.runOnUI(func() {
			if err != nil {
				dialog.ShowError(err, v.app.window)
				v.setStatus("Mongo command failed: %v", err)
				return
			}
			if strings.TrimSpace(output) != "" {
				v.detailJSON.Enable()
				v.detailJSON.SetText(output)
				v.detailJSON.Disable()
			} else if refresh {
				v.reloadSelection()
			}
			v.metaLabel.SetText(message)
			v.setStatus(message)
		})
	}(v.activeConfig, v.currentCollection, command)
}

func (v *explorerView) setStatus(format string, args ...interface{}) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	v.statusLabel.SetText(msg)
	v.app.setStatus(msg)
}

func (v *explorerView) clearState() {
	v.items = nil
	v.list.UnselectAll()
	v.list.Refresh()
	v.pgPreview = nil
	v.mongoPreview = nil
	v.previewTable.Refresh()
	v.mongoDocList.Refresh()
	v.detailJSON.Enable()
	v.detailJSON.SetText("")
	v.detailJSON.Disable()
	v.metaLabel.SetText("Select a profile to begin.")
	v.statusLabel.SetText("")
	v.activeConfig = nil
	v.activeProfile = nil
	v.currentCollection = ""
	v.currentTable = nil
	v.selectedRow = -1
	v.selectedDoc = -1
	v.addButton.Disable()
	v.duplicateButton.Disable()
	v.editButton.Disable()
	v.deleteButton.Disable()
}
