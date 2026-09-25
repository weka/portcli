package tui

import (
	"fmt"
	"time"

	"github.com/rivo/tview"
)

type flashLevel int

const (
	flashInfo flashLevel = iota
	flashWarn
	flashError
)

// flashTTL is how long a message stays. Errors linger because they are the
// ones worth reading twice.
func (l flashLevel) ttl() time.Duration {
	if l == flashError {
		return 10 * time.Second
	}
	return 5 * time.Second
}

func (l flashLevel) tag() string {
	switch l {
	case flashWarn:
		return "[" + tagWarn + "]"
	case flashError:
		return "[" + tagError + "]"
	default:
		return "[" + tagInfo + "]"
	}
}

// flashEntry is one recorded message.
type flashEntry struct {
	At      time.Time
	Level   flashLevel
	Message string
}

// flash is the one-line status area, plus a bounded history.
//
// The history exists because a TUI takes the scrollback away: a background
// refresh that failed twenty minutes ago has nowhere to be seen otherwise, so
// `:errors` can list what scrolled past.
type flash struct {
	view    *tview.TextView
	app     *tview.Application
	timer   *time.Timer
	history []flashEntry
}

const flashHistoryMax = 100

func newFlash(app *tview.Application) *flash {
	v := tview.NewTextView().SetDynamicColors(true)
	return &flash{view: v, app: app}
}

// show displays a message and records it. Safe only from the UI goroutine,
// which is where every caller runs — key handlers and refresher apply
// callbacks alike.
func (f *flash) show(level flashLevel, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	f.history = append(f.history, flashEntry{At: time.Now(), Level: level, Message: msg})
	if len(f.history) > flashHistoryMax {
		f.history = f.history[len(f.history)-flashHistoryMax:]
	}

	f.view.SetText(level.tag() + msg)
	if f.timer != nil {
		f.timer.Stop()
	}
	// The clear lands on the UI goroutine via QueueUpdateDraw because it fires
	// from the timer's own goroutine, not this one.
	f.timer = time.AfterFunc(level.ttl(), func() {
		f.app.QueueUpdateDraw(func() { f.view.SetText("") })
	})
}

func (f *flash) clear() {
	if f.timer != nil {
		f.timer.Stop()
	}
	f.view.SetText("")
}
