package explorer

import "github.com/rivo/tview"

func newModal(content tview.Primitive, width, height int) tview.Primitive {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 10
	}

	grid := tview.NewGrid().
		SetRows(0, height, 0).
		SetColumns(0, width, 0).
		AddItem(content, 1, 1, 1, 1, 0, 0, true)

	return grid
}
