package desktop

import (
	"fmt"
	"strings"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

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

	list  *widget.List
	items []explorerItem

	previewTable   *widget.Table
	previewColumns []string
	previewRows    [][]string
	jsonPreview    *widget.Entry

	metaLabel   *widget.Label
	statusLabel *widget.Label

	commandEntry *widget.Entry
	runButton    *widget.Button

	activeProfile     *profiles.Profile
	activeConfig      *config.Config
	dbType            string
	currentTable      *pgTable
	currentCollection string
}

func newExplorerView(app *App) *explorerView {
	view := &explorerView{
		app:           app,
		profileLookup: make(map[string]profiles.Profile),
	}

	view.profilePicker = widget.NewSelect([]string{}, func(value string) {
		view.onProfileSelected(value)
	})
	view.profilePicker.PlaceHolder = "Select profile…"

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
	view.list.OnUnselected = func(widget.ListItemID) {
		view.currentCollection = ""
		view.currentTable = nil
	}

	view.previewTable = widget.NewTable(
		view.tableSize,
		func() fyne.CanvasObject { return widget.NewLabel("") },
		view.updateTableCell,
	)

	view.jsonPreview = widget.NewMultiLineEntry()
	view.jsonPreview.SetMinRowsVisible(12)
	view.jsonPreview.Wrapping = fyne.TextWrapWord
	view.jsonPreview.Disable()

	view.metaLabel = widget.NewLabel("Pick a profile to inspect its tables or collections.")
	view.metaLabel.Wrapping = fyne.TextWrapWord

	view.statusLabel = widget.NewLabel("")
	view.statusLabel.TextStyle = fyne.TextStyle{Italic: true}

	view.commandEntry = widget.NewMultiLineEntry()
	view.commandEntry.SetMinRowsVisible(4)
	view.commandEntry.Wrapping = fyne.TextWrapWord
	view.commandEntry.PlaceHolder = "Postgres: SELECT * FROM public.users LIMIT 20\nMongo: insert {\"name\":\"alpha\"}"

	view.runButton = widget.NewButtonWithIcon("Run", theme.MediaPlayIcon(), func() {
		view.runCommand()
	})

	connectBtn := widget.NewButtonWithIcon("Connect", theme.ConfirmIcon(), func() {
		view.connectSelectedProfile()
	})
	refreshBtn := widget.NewButtonWithIcon("Reload", theme.ViewRefreshIcon(), func() {
		view.reloadInventory()
	})
	clearBtn := widget.NewButtonWithIcon("Clear", theme.ContentClearIcon(), func() {
		view.resetState()
		view.profilePicker.ClearSelected()
	})

	controls := container.NewGridWithColumns(4,
		container.NewVBox(widget.NewLabel("Profile"), view.profilePicker),
		connectBtn,
		refreshBtn,
		clearBtn,
	)

	listCard := widget.NewCard("Collections / Tables", "", container.NewMax(view.list))

	previewStack := container.NewStack(view.previewTable, view.jsonPreview)
	view.showJSON(false)
	view.showTable(false)

	previewCard := widget.NewCard("Preview", "", previewStack)
	detailsCard := widget.NewCard("Details", "", view.metaLabel)

	right := container.NewBorder(nil, detailsCard, nil, nil, previewCard)
	split := container.NewHSplit(listCard, right)
	split.SetOffset(0.32)

	commandHelper := widget.NewLabel("Command palette • Postgres accepts SQL, Mongo uses verb + JSON payload (insert/update/delete/find).")
	commandHelper.Wrapping = fyne.TextWrapWord
	commandArea := container.NewBorder(nil, nil, nil, view.runButton, view.commandEntry)

	footer := container.NewVBox(
		widget.NewSeparator(),
		commandHelper,
		commandArea,
		view.statusLabel,
	)

	view.content = container.NewBorder(controls, footer, nil, nil, split)
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

func (v *explorerView) tableSize() (int, int) {
	if len(v.previewColumns) == 0 {
		return 1, 1
	}
	return len(v.previewRows) + 1, len(v.previewColumns)
}

func (v *explorerView) updateTableCell(id widget.TableCellID, cell fyne.CanvasObject) {
	label := cell.(*widget.Label)
	if len(v.previewColumns) == 0 {
		label.SetText("")
		return
	}
	if id.Row == 0 {
		label.SetText(v.previewColumns[id.Col])
		label.TextStyle = fyne.TextStyle{Bold: true}
		return
	}
	label.TextStyle = fyne.TextStyle{}
	row := id.Row - 1
	if row < len(v.previewRows) && id.Col < len(v.previewColumns) {
		label.SetText(v.previewRows[row][id.Col])
	} else {
		label.SetText("")
	}
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
	}
	if len(options) == 0 {
		v.profilePicker.ClearSelected()
		v.resetState()
	}
}

func (v *explorerView) onProfileSelected(label string) {
	if strings.TrimSpace(label) == "" {
		return
	}
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
	v.setStatus("Connected to %s (%s).", profile.Name, strings.ToUpper(cfg.Database.Type))
	v.metaLabel.SetText(fmt.Sprintf("Connected to %s (%s). Loading objects…", profile.Name, strings.ToUpper(cfg.Database.Type)))
	v.loadInventory()
}

func (v *explorerView) connectSelectedProfile() {
	if v.profilePicker.Selected == "" {
		dialog.ShowInformation("Select profile", "Pick a saved profile first.", v.app.window)
		return
	}
	v.onProfileSelected(v.profilePicker.Selected)
}

func (v *explorerView) reloadInventory() {
	if v.activeConfig == nil {
		v.connectSelectedProfile()
		return
	}
	v.loadInventory()
}

func (v *explorerView) loadInventory() {
	if v.activeConfig == nil {
		return
	}
	v.items = nil
	v.list.Refresh()
	v.metaLabel.SetText("Loading schema information…")
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
					payload := table
					v.items[i] = explorerItem{
						Title:    fmt.Sprintf("%s.%s", table.Schema, table.Name),
						Subtitle: "Table",
						payload:  payload,
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
					v.items[i] = explorerItem{
						Title:    name,
						Subtitle: "Collection",
						payload:  name,
					}
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
	case pgTable:
		v.currentTable = &payload
		v.currentCollection = ""
		v.loadPostgresPreview(payload)
	case string:
		v.currentCollection = payload
		v.currentTable = nil
		v.loadMongoPreview(payload)
	}
}

func (v *explorerView) loadPostgresPreview(table pgTable) {
	if v.activeConfig == nil {
		return
	}
	v.metaLabel.SetText(fmt.Sprintf("Loading %s.%s …", table.Schema, table.Name))
	go func(cfg *config.Config, tbl pgTable) {
		columns, rows, err := fetchPostgresPreview(cfg, tbl, 200)
		v.app.runOnUI(func() {
			if err != nil {
				v.metaLabel.SetText(fmt.Sprintf("Preview failed: %v", err))
				v.setStatus("Preview failed: %v", err)
				return
			}
			v.previewColumns = columns
			v.previewRows = rows
			v.previewTable.Refresh()
			v.showTable(true)
			v.metaLabel.SetText(fmt.Sprintf("%s.%s • %d column(s) • %d row(s) previewed", tbl.Schema, tbl.Name, len(columns), len(rows)))
			v.setStatus("Preview refreshed.")
		})
	}(v.activeConfig, table)
}

func (v *explorerView) loadMongoPreview(collection string) {
	if v.activeConfig == nil {
		return
	}
	v.metaLabel.SetText(fmt.Sprintf("Loading %s …", collection))
	go func(cfg *config.Config, coll string) {
		text, count, err := fetchMongoPreview(cfg, coll, 50)
		v.app.runOnUI(func() {
			if err != nil {
				v.metaLabel.SetText(fmt.Sprintf("Preview failed: %v", err))
				v.setStatus("Preview failed: %v", err)
				return
			}
			v.jsonPreview.Enable()
			v.jsonPreview.SetText(text)
			v.jsonPreview.Disable()
			v.showJSON(true)
			v.metaLabel.SetText(fmt.Sprintf("%s • %d document(s) previewed", coll, count))
			v.setStatus("Preview refreshed.")
		})
	}(v.activeConfig, collection)
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
				v.previewColumns = columns
				v.previewRows = rows
				v.previewTable.Refresh()
				v.showTable(true)
				v.metaLabel.SetText(message)
			} else {
				v.metaLabel.SetText(message)
			}
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
				v.jsonPreview.Enable()
				v.jsonPreview.SetText(output)
				v.jsonPreview.Disable()
				v.showJSON(true)
			} else if refresh {
				v.loadMongoPreview(coll)
			}
			v.metaLabel.SetText(message)
			v.setStatus(message)
		})
	}(v.activeConfig, v.currentCollection, command)
}

func (v *explorerView) setStatus(format string, args ...interface{}) {
	if v.statusLabel == nil {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	v.statusLabel.SetText(msg)
	v.app.setStatus(msg)
}

func (v *explorerView) showTable(show bool) {
	if show {
		v.previewTable.Show()
	} else {
		v.previewTable.Hide()
	}
	v.jsonPreview.Hide()
}

func (v *explorerView) showJSON(show bool) {
	if show {
		v.jsonPreview.Show()
	} else {
		v.jsonPreview.Hide()
	}
	v.previewTable.Hide()
}

func (v *explorerView) resetState() {
	v.items = nil
	if v.list != nil {
		v.list.UnselectAll()
		v.list.Refresh()
	}
	v.previewColumns = nil
	v.previewRows = nil
	if v.previewTable != nil {
		v.previewTable.Refresh()
	}
	if v.jsonPreview != nil {
		v.jsonPreview.Enable()
		v.jsonPreview.SetText("")
		v.jsonPreview.Disable()
	}
	v.metaLabel.SetText("Select a profile to begin.")
	v.statusLabel.SetText("")
	v.activeProfile = nil
	v.activeConfig = nil
	v.currentTable = nil
	v.currentCollection = ""
	v.dbType = ""
}
