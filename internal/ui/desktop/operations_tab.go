package desktop

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kadirbelkuyu/DBRTS/internal/backup"
	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"github.com/kadirbelkuyu/DBRTS/internal/profiles"
	"github.com/kadirbelkuyu/DBRTS/pkg/logger"
)

type operationsView struct {
	app *App

	content fyne.CanvasObject

	profileLookup map[string]profiles.Profile

	transferSource  *widget.Select
	transferTarget  *widget.Select
	transferWorkers *widget.Entry
	transferBatch   *widget.Entry
	transferSchema  *widget.Check
	transferData    *widget.Check
	transferVerbose *widget.Check
	transferRun     *widget.Button

	backupProfile     *widget.Select
	backupDatabase    *widget.Entry
	backupFormat      *widget.Select
	backupCompression *widget.Entry
	backupSchemaOnly  *widget.Check
	backupDataOnly    *widget.Check
	backupOutput      *widget.Entry
	backupVerbose     *widget.Check
	backupRun         *widget.Button

	restoreProfile  *widget.Select
	restoreBackup   *widget.Entry
	restoreTargetDB *widget.Entry
	restoreCreate   *widget.Check
	restoreClean    *widget.Check
	restoreExit     *widget.Check
	restoreVerbose  *widget.Check
	restoreRun      *widget.Button

	logOutput *widget.Entry
}

func newOperationsView(app *App) *operationsView {
	view := &operationsView{
		app:           app,
		profileLookup: make(map[string]profiles.Profile),
	}

	view.transferSource = widget.NewSelect([]string{}, nil)
	view.transferTarget = widget.NewSelect([]string{}, nil)
	view.transferWorkers = widget.NewEntry()
	view.transferWorkers.SetText("4")
	view.transferBatch = widget.NewEntry()
	view.transferBatch.SetText("500")
	view.transferSchema = widget.NewCheck("Schema only", func(checked bool) {
		if checked {
			view.transferData.SetChecked(false)
		}
	})
	view.transferData = widget.NewCheck("Data only", func(checked bool) {
		if checked {
			view.transferSchema.SetChecked(false)
		}
	})
	view.transferVerbose = widget.NewCheck("Verbose logging", nil)
	view.transferRun = widget.NewButtonWithIcon("Run Transfer", theme.MediaPlayIcon(), func() {
		view.handleTransfer()
	})

	view.backupProfile = widget.NewSelect([]string{}, func(label string) {
		view.onBackupProfileChanged(label)
	})
	view.backupDatabase = widget.NewEntry()
	view.backupFormat = widget.NewSelect([]string{}, nil)
	view.backupCompression = widget.NewEntry()
	view.backupCompression.SetText("6")
	view.backupSchemaOnly = widget.NewCheck("Schema only", nil)
	view.backupDataOnly = widget.NewCheck("Data only", nil)
	view.backupOutput = widget.NewEntry()
	view.backupVerbose = widget.NewCheck("Verbose logging", nil)
	view.backupRun = widget.NewButtonWithIcon("Create Backup", theme.MediaPlayIcon(), func() {
		view.handleBackup()
	})

	view.restoreProfile = widget.NewSelect([]string{}, func(label string) {
		view.onRestoreProfileChanged(label)
	})
	view.restoreBackup = widget.NewEntry()
	view.restoreTargetDB = widget.NewEntry()
	view.restoreCreate = widget.NewCheck("Create DB when missing", nil)
	view.restoreClean = widget.NewCheck("Drop objects before restore", nil)
	view.restoreExit = widget.NewCheck("Stop on first error", func(checked bool) {})
	view.restoreExit.SetChecked(true)
	view.restoreVerbose = widget.NewCheck("Verbose logging", nil)
	view.restoreRun = widget.NewButtonWithIcon("Restore Backup", theme.MediaPlayIcon(), func() {
		view.handleRestore()
	})

	restoreBrowse := widget.NewButtonWithIcon("Browse…", theme.FolderOpenIcon(), func() {
		view.selectBackupFile()
	})

	view.logOutput = widget.NewMultiLineEntry()
	view.logOutput.SetMinRowsVisible(6)
	view.logOutput.Disable()

	transferForm := container.NewVBox(
		container.NewGridWithColumns(2,
			formRow("Source profile", view.transferSource),
			formRow("Target profile", view.transferTarget),
		),
		container.NewGridWithColumns(2,
			formRow("Workers", view.transferWorkers),
			formRow("Batch size", view.transferBatch),
		),
		container.NewGridWithColumns(3, view.transferSchema, view.transferData, view.transferVerbose),
		view.transferRun,
	)

	backupBasic := container.NewGridWithColumns(2,
		formRow("Profile", view.backupProfile),
		formRow("Database", view.backupDatabase),
	)
	backupOptions := container.NewGridWithColumns(3,
		formRow("Format", view.backupFormat),
		formRow("Compression", view.backupCompression),
		formRow("Output path", view.backupOutput),
	)

	backupForm := container.NewVBox(
		backupBasic,
		backupOptions,
		container.NewGridWithColumns(3, view.backupSchemaOnly, view.backupDataOnly, view.backupVerbose),
		view.backupRun,
	)

	restoreForm := container.NewVBox(
		container.NewGridWithColumns(2,
			formRow("Profile", view.restoreProfile),
			formRow("Backup file", container.NewBorder(nil, nil, nil, restoreBrowse, view.restoreBackup)),
		),
		container.NewGridWithColumns(2,
			formRow("Target database", view.restoreTargetDB),
			formRow("Options", container.NewGridWithColumns(1, view.restoreCreate, view.restoreClean, view.restoreExit, view.restoreVerbose)),
		),
		view.restoreRun,
	)

	view.content = container.NewVScroll(container.NewVBox(
		widget.NewCard("Transfer", "Move data between saved PostgreSQL or MongoDB environments.", transferForm),
		widget.NewCard("Backup", "Capture portable backups with compression and format controls.", backupForm),
		widget.NewCard("Restore", "Apply an archived backup to a selected profile.", restoreForm),
		widget.NewCard("Activity Log", "", container.NewMax(view.logOutput)),
	))

	return view
}

func (a *App) buildOperationsTab() fyne.CanvasObject {
	if a.operations == nil {
		a.operations = newOperationsView(a)
	}
	return a.operations.canvas()
}

func (v *operationsView) canvas() fyne.CanvasObject {
	return v.content
}

func (v *operationsView) updateProfiles(list []profiles.Profile) {
	v.profileLookup = make(map[string]profiles.Profile)
	options := make([]string, len(list))
	for i, profile := range list {
		label := fmt.Sprintf("%s (%s)", profile.Name, strings.ToUpper(profile.Type))
		options[i] = label
		v.profileLookup[label] = profile
	}
	for _, selectBox := range []*widget.Select{v.transferSource, v.transferTarget, v.backupProfile, v.restoreProfile} {
		selectBox.Options = options
		selectBox.Refresh()
	}
}

func (v *operationsView) loadConfig(label string) (*config.Config, profiles.Profile, error) {
	profile, ok := v.profileLookup[label]
	if !ok {
		return nil, profiles.Profile{}, fmt.Errorf("profile %s not found", label)
	}
	cfg, err := config.LoadConfig(profile.Path)
	if err != nil {
		return nil, profiles.Profile{}, fmt.Errorf("load config %s: %w", profile.Name, err)
	}
	return cfg, profile, nil
}

func (v *operationsView) handleTransfer() {
	if v.transferSource.Selected == "" || v.transferTarget.Selected == "" {
		dialog.ShowInformation("Select profiles", "Choose both source and target profiles.", v.app.window)
		return
	}

	sourceCfg, sourceProfile, err := v.loadConfig(v.transferSource.Selected)
	if err != nil {
		dialog.ShowError(err, v.app.window)
		return
	}
	targetCfg, targetProfile, err := v.loadConfig(v.transferTarget.Selected)
	if err != nil {
		dialog.ShowError(err, v.app.window)
		return
	}
	if sourceProfile.Type != targetProfile.Type {
		dialog.ShowInformation("Type mismatch", "Source and target must be the same engine.", v.app.window)
		return
	}

	workers, err := parseIntEntry(v.transferWorkers, 4)
	if err != nil {
		dialog.ShowError(fmt.Errorf("workers: %w", err), v.app.window)
		return
	}
	batch, err := parseIntEntry(v.transferBatch, 500)
	if err != nil {
		dialog.ShowError(fmt.Errorf("batch size: %w", err), v.app.window)
		return
	}

	schemaOnly := v.transferSchema.Checked
	dataOnly := v.transferData.Checked
	verbose := v.transferVerbose.Checked

	v.execute("Transfer", v.transferRun, func() error {
		return v.app.service.Transfer(sourceCfg, targetCfg, schemaOnly, dataOnly, workers, batch, verbose)
	})
}

func (v *operationsView) handleBackup() {
	if v.backupProfile.Selected == "" {
		dialog.ShowInformation("Select profile", "Choose which profile to back up.", v.app.window)
		return
	}

	cfg, profile, err := v.loadConfig(v.backupProfile.Selected)
	if err != nil {
		dialog.ShowError(err, v.app.window)
		return
	}

	dbName := strings.TrimSpace(v.backupDatabase.Text)
	if dbName == "" {
		dbName = strings.TrimSpace(cfg.Database.Database)
	}
	if dbName == "" {
		dialog.ShowInformation("Database required", "Provide a database name to back up.", v.app.window)
		return
	}

	format := v.backupFormat.Selected
	if strings.TrimSpace(format) == "" {
		format = defaultBackupFormat(profile.Type)
	}
	compression, err := parseIntEntry(v.backupCompression, defaultCompression(profile.Type))
	if err != nil {
		dialog.ShowError(fmt.Errorf("compression: %w", err), v.app.window)
		return
	}

	options := backup.BackupOptions{
		Format:      format,
		Compression: compression,
		SchemaOnly:  profile.Type == "postgres" && v.backupSchemaOnly.Checked,
		DataOnly:    profile.Type == "postgres" && v.backupDataOnly.Checked,
		OutputPath:  strings.TrimSpace(v.backupOutput.Text),
		Verbose:     v.backupVerbose.Checked,
	}

	v.execute("Backup", v.backupRun, func() error {
		log := logger.NewLogger(v.backupVerbose.Checked)
		service, err := backup.NewService(cfg, log)
		if err != nil {
			return err
		}
		if err := service.Connect(); err != nil {
			return err
		}
		defer service.Close()

		metadata, err := service.CreateBackup(dbName, options)
		if err != nil {
			return err
		}
		v.appendLog("Backup stored at %s (%d bytes)", metadata.Location, metadata.BackupSize)
		return nil
	})
}

func (v *operationsView) handleRestore() {
	if v.restoreProfile.Selected == "" {
		dialog.ShowInformation("Select profile", "Choose where to restore the backup.", v.app.window)
		return
	}

	cfg, profile, err := v.loadConfig(v.restoreProfile.Selected)
	if err != nil {
		dialog.ShowError(err, v.app.window)
		return
	}

	path := strings.TrimSpace(v.restoreBackup.Text)
	if path == "" {
		dialog.ShowInformation("Backup path", "Provide the backup file/directory path.", v.app.window)
		return
	}

	targetDB := strings.TrimSpace(v.restoreTargetDB.Text)
	if targetDB == "" {
		targetDB = strings.TrimSpace(cfg.Database.Database)
	}
	if targetDB == "" {
		dialog.ShowInformation("Target database", "Provide a target database name.", v.app.window)
		return
	}

	options := backup.RestoreOptions{
		BackupPath:     path,
		TargetDatabase: targetDB,
		CreateDatabase: v.restoreCreate.Checked || profile.Type == "mongo",
		CleanFirst:     v.restoreClean.Checked,
		Verbose:        v.restoreVerbose.Checked,
		ExitOnError:    v.restoreExit.Checked,
	}

	v.execute("Restore", v.restoreRun, func() error {
		log := logger.NewLogger(v.restoreVerbose.Checked)
		service, err := backup.NewService(cfg, log)
		if err != nil {
			return err
		}
		if err := service.Connect(); err != nil {
			return err
		}
		defer service.Close()

		if err := service.RestoreBackup(options); err != nil {
			return err
		}
		v.appendLog("Restore completed for %s.", targetDB)
		return nil
	})
}

func (v *operationsView) onBackupProfileChanged(label string) {
	profile, ok := v.profileLookup[label]
	if !ok {
		return
	}
	cfg, err := config.LoadConfig(profile.Path)
	if err == nil && strings.TrimSpace(cfg.Database.Database) != "" {
		v.backupDatabase.SetText(cfg.Database.Database)
	}
	switch profile.Type {
	case "mongo":
		v.backupFormat.Options = []string{"archive"}
		v.backupFormat.SetSelected("archive")
		v.backupSchemaOnly.Disable()
		v.backupDataOnly.Disable()
		v.backupCompression.SetText("1")
	default:
		v.backupFormat.Options = []string{"custom", "sql", "tar", "directory"}
		v.backupFormat.SetSelected("custom")
		v.backupSchemaOnly.Enable()
		v.backupDataOnly.Enable()
		v.backupCompression.SetText("6")
	}
	v.backupFormat.Refresh()
}

func (v *operationsView) onRestoreProfileChanged(label string) {
	profile, ok := v.profileLookup[label]
	if !ok {
		return
	}
	cfg, err := config.LoadConfig(profile.Path)
	if err == nil && strings.TrimSpace(cfg.Database.Database) != "" {
		v.restoreTargetDB.SetText(cfg.Database.Database)
	}
	if profile.Type == "mongo" {
		v.restoreClean.SetText("Drop collections before restore")
	} else {
		v.restoreClean.SetText("Drop objects before restore")
	}
}

func (v *operationsView) selectBackupFile() {
	dlg := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		v.restoreBackup.SetText(reader.URI().Path())
	}, v.app.window)
	dlg.SetFilter(storage.NewExtensionFileFilter([]string{".dump", ".sql", ".archive", ".tar", ".gz"}))
	dlg.Show()
}

func (v *operationsView) execute(action string, button *widget.Button, task func() error) {
	button.Disable()
	v.appendLog("%s started.", action)
	go func() {
		err := task()
		v.app.runOnUI(func() {
			button.Enable()
			if err != nil {
				dialog.ShowError(err, v.app.window)
				v.appendLog("%s failed: %v", action, err)
			} else {
				v.appendLog("%s completed successfully.", action)
			}
		})
	}()
}

func (v *operationsView) appendLog(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, message)
	v.logOutput.Enable()
	if strings.TrimSpace(v.logOutput.Text) == "" {
		v.logOutput.SetText(entry)
	} else {
		v.logOutput.SetText(v.logOutput.Text + "\n" + entry)
	}
	v.logOutput.Disable()
	v.app.setStatus(message)
}

func parseIntEntry(entry *widget.Entry, fallback int) (int, error) {
	value := strings.TrimSpace(entry.Text)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("expecting a number")
	}
	if parsed <= 0 {
		return fallback, nil
	}
	return parsed, nil
}

func defaultBackupFormat(dbType string) string {
	if dbType == "mongo" {
		return "archive"
	}
	return "custom"
}

func defaultCompression(dbType string) int {
	if dbType == "mongo" {
		return 1
	}
	return 6
}
