package desktop

import (
	"fmt"
	"sort"
	"strings"

	fyne "fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/kadirbelkuyu/DBRTS/internal/app"
	"github.com/kadirbelkuyu/DBRTS/internal/profiles"
)

const defaultConfigDir = "configs"

// Run starts the desktop experience for DBRTS using the provided config directory.
func Run(configDir string) error {
	dir := strings.TrimSpace(configDir)
	if dir == "" {
		dir = defaultConfigDir
	}

	desktop := &App{
		configDir: dir,
		manager:   profiles.NewManager(dir),
		service:   app.NewService(),
	}

	return desktop.Run()
}

// App represents the desktop UI controller.
type App struct {
	configDir string
	manager   *profiles.Manager
	service   *app.Service

	app    fyne.App
	window fyne.Window

	status *widget.Label
	tabs   *container.AppTabs

	profileItems []profiles.Profile
	profileList  *widget.List
	profileForm  *profileEditor

	explorer   *explorerView
	operations *operationsView
}

// Run bootstraps the fyne application and blocks until the window is closed.
func (a *App) Run() error {
	a.app = fyneapp.NewWithID("github.com/kadirbelkuyu/dbrts/desktop")
	a.window = a.app.NewWindow("DBRTS Studio")
	a.window.Resize(fyne.NewSize(1280, 800))
	a.app.Settings().SetTheme(theme.DarkTheme())

	a.window.SetContent(a.buildShell())
	a.refreshProfiles()
	a.window.ShowAndRun()
	return nil
}

func (a *App) buildShell() fyne.CanvasObject {
	header := a.buildHeader()
	status := a.buildStatusBar()

	a.profileForm = newProfileEditor(a)
	a.explorer = newExplorerView(a)
	a.operations = newOperationsView(a)

	a.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Profiles", theme.AccountIcon(), a.buildProfilesTab()),
		container.NewTabItemWithIcon("Explorer", theme.ViewRefreshIcon(), a.buildExplorerTab()),
		container.NewTabItemWithIcon("Operations", theme.StorageIcon(), a.buildOperationsTab()),
	)
	a.tabs.SetTabLocation(container.TabLocationLeading)

	return container.NewBorder(header, status, nil, nil, a.tabs)
}

func (a *App) buildHeader() fyne.CanvasObject {
	title := widget.NewLabelWithStyle("DBRTS Studio", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.TextStyle = fyne.TextStyle{Bold: true}
	subtitle := widget.NewLabel("Desktop companion for PostgreSQL & MongoDB automation workflows.")
	subtitle.Wrapping = fyne.TextWrapWord

	return container.NewVBox(title, subtitle, widget.NewSeparator())
}

func (a *App) buildStatusBar() fyne.CanvasObject {
	a.status = widget.NewLabel("Ready.")
	return container.NewBorder(nil, nil, widget.NewLabel("Status"), nil, a.status)
}

func (a *App) setStatus(format string, args ...interface{}) {
	if a.status == nil {
		return
	}
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	a.runOnUI(func() {
		a.status.SetText(msg)
	})
}

func (a *App) runOnUI(fn func()) {
	if fn == nil {
		return
	}
	if a.app == nil {
		fn()
		return
	}
	fyne.Do(fn)
}

func (a *App) refreshProfiles() {
	profilesList, err := a.manager.List("")
	if err != nil {
		a.setStatus("Failed to load profiles: %v", err)
		return
	}
	sort.SliceStable(profilesList, func(i, j int) bool {
		return strings.ToLower(profilesList[i].Name) < strings.ToLower(profilesList[j].Name)
	})
	a.profileItems = profilesList
	if a.profileList != nil {
		a.profileList.Refresh()
		if len(a.profileItems) == 0 {
			a.profileList.UnselectAll()
		}
	}
	if a.explorer != nil {
		a.explorer.updateProfiles(profilesList)
	}
	if a.operations != nil {
		a.operations.updateProfiles(profilesList)
	}
	if len(profilesList) == 0 && a.profileForm != nil {
		a.profileForm.reset(nil, nil)
	}
}
