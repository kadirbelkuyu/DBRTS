package explorer

import "github.com/rivo/tview"

func queueUpdate(app *tview.Application, fn func()) {
	if app == nil {
		fn()
		return
	}

	if err := app.QueueUpdateDraw(fn); err != nil {
		fn()
	}
}
