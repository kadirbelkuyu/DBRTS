package progress

import (
	"fmt"
	"time"

	"github.com/schollz/progressbar/v3"
)

type Bar struct {
	*progressbar.ProgressBar
}

func NewBar(max int64, description string) *Bar {
	bar := progressbar.NewOptions64(max,
		progressbar.OptionSetDescription(description),
		progressbar.OptionSetWidth(50),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "=",
			SaucerHead:    ">",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionOnCompletion(func() {
			fmt.Println()
		}),
	)

	return &Bar{ProgressBar: bar}
}

func (b *Bar) Increment() {
	b.Add(1)
}

func (b *Bar) IncrementBy(amount int64) {
	b.Add64(amount)
}

func (b *Bar) Finish() {
	if b.ProgressBar == nil {
		return
	}
	b.ProgressBar.Finish()
}
