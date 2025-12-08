package desktop

import (
	fyne "fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func placeholder(message string) fyne.CanvasObject {
	label := widget.NewLabel(message)
	label.Alignment = fyne.TextAlignCenter
	return container.NewCenter(label)
}
