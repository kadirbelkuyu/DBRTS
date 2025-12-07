package desktop

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/database"
	"github.com/kadirbelkuyu/DBRTS/internal/profiles"
)

func (a *App) buildProfilesTab() fyne.CanvasObject {
	if a.profileForm == nil {
		a.profileForm = newProfileEditor(a)
	}

	a.profileSearch = widget.NewEntry()
	a.profileSearch.SetPlaceHolder("🔍 Search profiles...")
	a.profileSearch.OnChanged = func(text string) {
		a.filterProfileList(text)
	}

	a.profileList = widget.NewList(
		func() int {
			if len(a.profileFiltered) > 0 {
				return len(a.profileFiltered)
			}
			return len(a.profileItems)
		},
		func() fyne.CanvasObject {
			icon := widget.NewIcon(theme.AccountIcon())
			name := widget.NewLabel("")
			name.TextStyle = fyne.TextStyle{Bold: true}
			meta := widget.NewLabel("")
			meta.TextStyle = fyne.TextStyle{Italic: true}
			header := container.NewHBox(icon, name)
			return container.NewVBox(header, meta)
		},
		func(id widget.ListItemID, item fyne.CanvasObject) {
			index := a.getProfileActualIndex(int(id))
			if index < 0 || index >= len(a.profileItems) {
				return
			}
			profile := a.profileItems[index]
			box := item.(*fyne.Container)
			if len(box.Objects) >= 2 {
				headerBox := box.Objects[0].(*fyne.Container)
				if len(headerBox.Objects) >= 2 {
					iconWidget := headerBox.Objects[0].(*widget.Icon)
					nameLabel := headerBox.Objects[1].(*widget.Label)
					if strings.ToLower(profile.Type) == "mongo" {
						iconWidget.SetResource(theme.StorageIcon())
					} else {
						iconWidget.SetResource(theme.ComputerIcon())
					}
					nameLabel.SetText(profile.Name)
				}
				box.Objects[1].(*widget.Label).SetText(profileMeta(profile))
			}
		},
	)
	a.profileList.OnSelected = func(id widget.ListItemID) {
		index := a.getProfileActualIndex(int(id))
		a.handleProfileSelection(index)
	}
	a.profileList.OnUnselected = func(id widget.ListItemID) {
		a.profileForm.reset(nil, nil)
	}

	deleteBtn := widget.NewButtonWithIcon("Delete", theme.DeleteIcon(), func() {
		a.deleteSelectedProfile()
	})
	deleteBtn.Importance = widget.DangerImportance

	duplicateBtn := widget.NewButtonWithIcon("Duplicate", theme.ContentCopyIcon(), func() {
		a.duplicateSelectedProfile()
	})

	listActions := container.NewGridWithColumns(2, duplicateBtn, deleteBtn)
	listWithSearch := container.NewBorder(a.profileSearch, listActions, nil, nil, a.profileList)

	listCard := widget.NewCard("Saved Profiles", "", listWithSearch)
	split := container.NewHSplit(
		listCard,
		a.profileForm.canvas(),
	)
	split.SetOffset(0.35)

	return split
}

func (a *App) handleProfileSelection(index int) {
	if index < 0 || index >= len(a.profileItems) {
		a.profileSelectedIdx = -1
		return
	}

	a.profileSelectedIdx = index
	profile := a.profileItems[index]
	cfg, err := config.LoadConfig(profile.Path)
	if err != nil {
		a.setStatus("Failed to load profile %s: %v", profile.Name, err)
		dialog.ShowError(fmt.Errorf("load profile %s: %w", profile.Name, err), a.window)
		return
	}

	profileCopy := profile
	a.profileForm.reset(&profileCopy, cfg)
	a.setStatus("Loaded profile %s", profile.Name)
}

func (a *App) filterProfileList(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		a.profileFiltered = nil
		a.profileList.Refresh()
		return
	}

	a.profileFiltered = make([]int, 0)
	for i, profile := range a.profileItems {
		if strings.Contains(strings.ToLower(profile.Name), query) ||
			strings.Contains(strings.ToLower(profile.Type), query) {
			a.profileFiltered = append(a.profileFiltered, i)
		}
	}
	a.profileList.UnselectAll()
	a.profileList.Refresh()
}

func (a *App) getProfileActualIndex(displayIndex int) int {
	if len(a.profileFiltered) > 0 {
		if displayIndex >= 0 && displayIndex < len(a.profileFiltered) {
			return a.profileFiltered[displayIndex]
		}
		return -1
	}
	return displayIndex
}

func (a *App) deleteSelectedProfile() {
	index := a.profileSelectedIdx
	if index < 0 || index >= len(a.profileItems) {
		dialog.ShowInformation("Select Profile", "Please select a profile to delete.", a.window)
		return
	}

	profile := a.profileItems[index]
	content := fmt.Sprintf("Are you sure you want to delete '%s'?\n\nType: %s\nPath: %s",
		profile.Name, strings.ToUpper(profile.Type), profile.Path)

	dialog.ShowConfirm("Delete Profile", content, func(ok bool) {
		if !ok {
			return
		}
		if err := a.manager.Delete(profile.Name); err != nil {
			dialog.ShowError(fmt.Errorf("delete profile: %w", err), a.window)
			a.setStatus("Failed to delete profile: %v", err)
			return
		}
		a.setStatus("Deleted profile %s", profile.Name)
		a.refreshProfiles()
		a.profileForm.reset(nil, nil)
	}, a.window)
}

func (a *App) duplicateSelectedProfile() {
	index := a.profileSelectedIdx
	if index < 0 || index >= len(a.profileItems) {
		dialog.ShowInformation("Select Profile", "Please select a profile to duplicate.", a.window)
		return
	}

	profile := a.profileItems[index]
	cfg, err := config.LoadConfig(profile.Path)
	if err != nil {
		dialog.ShowError(fmt.Errorf("load profile: %w", err), a.window)
		return
	}

	newName := profile.Name + "-copy"
	entry := widget.NewEntry()
	entry.SetText(newName)

	dialog.ShowForm("Duplicate Profile", "Create", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("New name", entry)},
		func(ok bool) {
			if !ok {
				return
			}
			newAlias := strings.TrimSpace(entry.Text)
			if newAlias == "" {
				dialog.ShowError(fmt.Errorf("profile name cannot be empty"), a.window)
				return
			}
			newProfile, err := a.manager.Save(newAlias, cfg)
			if err != nil {
				dialog.ShowError(fmt.Errorf("save duplicate: %w", err), a.window)
				return
			}
			a.setStatus("Created duplicate profile %s", newProfile.Name)
			a.refreshProfiles()
		}, a.window)
}

func profileMeta(p profiles.Profile) string {
	typ := strings.ToUpper(p.Type)
	if p.Modified.IsZero() {
		return typ
	}
	return fmt.Sprintf("%s • %s", typ, humanizeDuration(time.Since(p.Modified)))
}

func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return "moments ago"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	if d < 7*24*time.Hour {
		return fmt.Sprintf("%d d ago", int(d.Hours()/24))
	}
	return fmt.Sprintf("on %s", time.Now().Add(-d).Format("02 Jan"))
}

type profileEditor struct {
	app *App

	aliasEntry    *widget.Entry
	engineSelect  *widget.Select
	hostEntry     *widget.Entry
	portEntry     *widget.Entry
	databaseEntry *widget.Entry
	userEntry     *widget.Entry
	passwordEntry *widget.Entry
	sslSelect     *widget.Select
	uriEntry      *widget.Entry
	authDBEntry   *widget.Entry

	infoLabel *widget.Label
	content   fyne.CanvasObject

	currentProfile *profiles.Profile
}

func newProfileEditor(app *App) *profileEditor {
	editor := &profileEditor{
		app: app,
	}

	editor.aliasEntry = widget.NewEntry()
	editor.aliasEntry.PlaceHolder = "prod-replica"

	editor.engineSelect = widget.NewSelect([]string{"PostgreSQL", "MongoDB"}, func(value string) {
		editor.toggleEngine(value)
	})

	editor.hostEntry = widget.NewEntry()
	editor.hostEntry.PlaceHolder = "localhost"

	editor.portEntry = widget.NewEntry()
	editor.portEntry.PlaceHolder = "5432 / 27017"

	editor.databaseEntry = widget.NewEntry()
	editor.databaseEntry.PlaceHolder = "database name"

	editor.userEntry = widget.NewEntry()
	editor.userEntry.PlaceHolder = "db user"

	editor.passwordEntry = widget.NewPasswordEntry()
	editor.passwordEntry.PlaceHolder = "secret"

	editor.sslSelect = widget.NewSelect([]string{"disable", "require", "verify-ca", "verify-full"}, nil)
	editor.sslSelect.SetSelected("disable")

	editor.uriEntry = widget.NewEntry()
	editor.uriEntry.PlaceHolder = "mongodb://user:pass@host:27017/db"

	editor.authDBEntry = widget.NewEntry()
	editor.authDBEntry.PlaceHolder = "admin"

	editor.infoLabel = widget.NewLabel("Select or create a profile to get started.")

	form := container.NewVBox(
		formRow("Alias", editor.aliasEntry),
		formRow("Engine", editor.engineSelect),
		widget.NewSeparator(),
		formRow("Host", editor.hostEntry),
		formRow("Port", editor.portEntry),
		formRow("Database", editor.databaseEntry),
		formRow("Username", editor.userEntry),
		formRow("Password", editor.passwordEntry),
		formRow("SSL Mode (Postgres)", editor.sslSelect),
		widget.NewSeparator(),
		formRow("Mongo URI (optional)", editor.uriEntry),
		formRow("Mongo Auth DB", editor.authDBEntry),
	)

	buttons := container.NewGridWithColumns(3,
		widget.NewButtonWithIcon("New Profile", theme.ContentAddIcon(), func() {
			editor.reset(nil, nil)
		}),
		widget.NewButtonWithIcon("Test Connection", theme.ConfirmIcon(), func() {
			editor.testConnection()
		}),
		widget.NewButtonWithIcon("Save Profile", theme.DocumentSaveIcon(), func() {
			editor.saveProfile()
		}),
	)

	editor.content = container.NewBorder(
		nil,
		container.NewVBox(editor.infoLabel, buttons),
		nil,
		nil,
		container.NewVScroll(form),
	)

	editor.reset(nil, nil)
	return editor
}

func (p *profileEditor) canvas() fyne.CanvasObject {
	return widget.NewCard("Profile Editor", "Define connection details compatible with the CLI workflows.", p.content)
}

func formRow(label string, control fyne.CanvasObject) fyne.CanvasObject {
	title := widget.NewLabel(label)
	title.Alignment = fyne.TextAlignLeading
	return container.NewVBox(title, control)
}

func (p *profileEditor) toggleEngine(display string) {
	switch strings.ToLower(display) {
	case "mongodb", "mongo":
		p.sslSelect.Disable()
		if p.portEntry.Text == "" || p.portEntry.Text == "5432" {
			p.portEntry.SetText("27017")
		}
	default:
		p.sslSelect.Enable()
		if p.portEntry.Text == "" || p.portEntry.Text == "27017" {
			p.portEntry.SetText("5432")
		}
	}
}

func (p *profileEditor) reset(profile *profiles.Profile, cfg *config.Config) {
	p.currentProfile = profile

	if cfg == nil {
		cfg = &config.Config{
			Database: config.DatabaseConfig{
				Type:    "postgres",
				Host:    "localhost",
				Port:    5432,
				SSLMode: "disable",
			},
		}
	}

	p.aliasEntry.SetText("")
	if profile != nil {
		p.aliasEntry.SetText(profile.Name)
	}

	displayType := renderEngine(cfg.Database.Type)
	if displayType == "" {
		displayType = "PostgreSQL"
	}
	p.engineSelect.SetSelected(displayType)

	if cfg.Database.Host != "" {
		p.hostEntry.SetText(cfg.Database.Host)
	} else {
		p.hostEntry.SetText("localhost")
	}

	if cfg.Database.Port > 0 {
		p.portEntry.SetText(strconv.Itoa(cfg.Database.Port))
	} else if cfg.Database.Type == "mongo" {
		p.portEntry.SetText("27017")
	} else {
		p.portEntry.SetText("5432")
	}

	p.databaseEntry.SetText(cfg.Database.Database)
	p.userEntry.SetText(cfg.Database.Username)
	p.passwordEntry.SetText(cfg.Database.Password)
	if cfg.Database.SSLMode != "" {
		p.sslSelect.SetSelected(cfg.Database.SSLMode)
	} else {
		p.sslSelect.SetSelected("disable")
	}

	p.uriEntry.SetText(cfg.Database.URI)
	p.authDBEntry.SetText(cfg.Database.AuthDatabase)

	if profile != nil {
		p.infoLabel.SetText(fmt.Sprintf("Editing %s (%s)", profile.Name, strings.ToUpper(cfg.Database.Type)))
	} else {
		p.infoLabel.SetText("New profile. Fill the fields and click Save.")
	}
}

func (p *profileEditor) saveProfile() {
	alias, cfg, err := p.readForm()
	if err != nil {
		dialog.ShowError(err, p.app.window)
		p.app.setStatus("Cannot save profile: %v", err)
		return
	}

	profile, err := p.app.manager.Save(alias, cfg)
	if err != nil {
		dialog.ShowError(fmt.Errorf("save profile: %w", err), p.app.window)
		p.app.setStatus("Cannot save profile: %v", err)
		return
	}

	profileCopy := profile
	p.reset(&profileCopy, cfg)
	p.app.refreshProfiles()
	p.app.setStatus("Saved profile %s", profile.Name)
	p.infoLabel.SetText(fmt.Sprintf("Profile %s saved.", profile.Name))
}

func (p *profileEditor) testConnection() {
	_, cfg, err := p.readForm()
	if err != nil {
		dialog.ShowError(err, p.app.window)
		p.app.setStatus("Cannot test connection: %v", err)
		return
	}

	p.infoLabel.SetText("Testing connection…")
	p.app.setStatus("Testing %s connection…", strings.ToUpper(cfg.Database.Type))

	go func() {
		err := pingDatabase(cfg)
		p.app.runOnUI(func() {
			if err != nil {
				dialog.ShowError(err, p.app.window)
				p.infoLabel.SetText(fmt.Sprintf("Connection failed: %v", err))
				p.app.setStatus("Connection failed: %v", err)
				return
			}
			p.infoLabel.SetText("Connection successful.")
			p.app.setStatus("Connection succeeded.")
		})
	}()
}

func (p *profileEditor) readForm() (string, *config.Config, error) {
	cfg, err := p.buildConfig()
	if err != nil {
		return "", nil, err
	}
	alias := strings.TrimSpace(p.aliasEntry.Text)
	return alias, cfg, nil
}

func (p *profileEditor) buildConfig() (*config.Config, error) {
	engine := normalizeEngineSelection(p.engineSelect.Selected)
	if engine == "" {
		return nil, errors.New("select a database engine")
	}

	port := 0
	portText := strings.TrimSpace(p.portEntry.Text)
	if portText != "" {
		parsed, err := strconv.Atoi(portText)
		if err != nil {
			return nil, fmt.Errorf("port must be a valid number")
		}
		port = parsed
	} else if engine == "postgres" {
		port = 5432
	} else {
		port = 27017
	}

	host := strings.TrimSpace(p.hostEntry.Text)
	uri := strings.TrimSpace(p.uriEntry.Text)
	if host == "" && uri == "" {
		return nil, errors.New("host or URI is required")
	}

	databaseName := strings.TrimSpace(p.databaseEntry.Text)
	if databaseName == "" {
		return nil, errors.New("database name is required")
	}

	cfg := &config.Config{
		Database: config.DatabaseConfig{
			Type:         engine,
			Host:         host,
			Port:         port,
			Database:     databaseName,
			Username:     strings.TrimSpace(p.userEntry.Text),
			Password:     strings.TrimSpace(p.passwordEntry.Text),
			SSLMode:      p.sslSelect.Selected,
			URI:          uri,
			AuthDatabase: strings.TrimSpace(p.authDBEntry.Text),
		},
	}

	if cfg.Database.Type == "postgres" && cfg.Database.Username == "" {
		return nil, errors.New("username is required for PostgreSQL")
	}

	return cfg, nil
}

func normalizeEngineSelection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "postgresql", "postgres":
		return "postgres"
	case "mongodb", "mongo":
		return "mongo"
	default:
		return ""
	}
}

func renderEngine(dbType string) string {
	switch strings.ToLower(strings.TrimSpace(dbType)) {
	case "mongo", "mongodb":
		return "MongoDB"
	default:
		return "PostgreSQL"
	}
}

func pingDatabase(cfg *config.Config) error {
	switch cfg.Database.Type {
	case "postgres":
		conn, err := database.NewConnection(cfg)
		if err != nil {
			return err
		}
		defer conn.Close()
		return nil
	case "mongo":
		uri := cfg.GetMongoURI()
		if strings.TrimSpace(uri) == "" {
			return errors.New("mongo URI could not be derived from config")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			return fmt.Errorf("connect failed: %w", err)
		}
		defer client.Disconnect(context.Background())
		if err := client.Ping(ctx, nil); err != nil {
			return fmt.Errorf("ping failed: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
}
