package tmp

import (
	"testing"

	"github.com/rivo/tview"
)

func TestQueueUpdateDrawBeforeRun(t *testing.T) {
	app := tview.NewApplication()
	if err := app.QueueUpdateDraw(func() {}); err == nil {
		t.Fatalf("expected error when QueueUpdateDraw called before Run")
	}
}
