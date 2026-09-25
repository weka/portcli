package tui

import (
	"context"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// View is anything the stack can show. Most views are a table over a
// Resource, but logs and describe are not tabular, which is why the stack is
// defined over this rather than over Resource.
type View interface {
	// ID keys the view in the body's page set; pushing the same ID twice
	// reuses the existing page and its scroll position.
	ID() string
	// Title is shown in the breadcrumb.
	Title() string
	Primitive() tview.Primitive
	// Start begins refreshing. Only the top of the stack is started, so N
	// stacked views never poll concurrently.
	Start(ctx context.Context)
	// Stop cancels refreshing. It must not block; see refresher.stop.
	Stop()
	// Ops are the operations active while this view is on top.
	Ops() []Op
}

// keyHandler is an optional View extension for keys that are not row
// operations — filtering log lines, toggling follow, and so on.
type keyHandler interface {
	HandleKey(ev *tcell.EventKey) bool
}
