package tui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// stolenKeys are keys tview.Table binds internally that we take over anyway.
//
// Table.InputHandler wires h and l to horizontal column scrolling with no
// guard on whether columns are selectable, so k9s's `l` for logs would
// otherwise never reach us. Horizontal scrolling stays available on the arrow
// keys. Everything else the Table binds — j/k, g/G, ctrl-f/ctrl-b, Home/End,
// PgUp/PgDn — is left to it, because its handling is already what we want.
var stolenKeys = map[rune]bool{'h': true, 'l': true}

// globalCapture is the single entry point for keys.
//
// SetInputCapture runs before the focused primitive, so the focus guard is not
// optional: without it, typing "d" into a filter field would trigger Describe
// and ":" could never be typed at all.
func (a *App) globalCapture(ev *tcell.EventKey) *tcell.EventKey {
	switch a.app.GetFocus().(type) {
	case *tview.InputField, *tview.TextArea, *tview.DropDown, *tview.Button, *tview.Checkbox:
		// Anything that consumes text owns the keyboard.
		return ev
	}

	// An overlay is modal: let it have the keys, except Escape which closes it.
	if name, _ := a.pages.GetFrontPage(); name != "main" {
		if ev.Key() == tcell.KeyEscape {
			a.closeOverlay(name)
			return nil
		}
		return ev
	}

	if handled := a.handleGlobalKey(ev); handled {
		return nil
	}

	// View-specific keys that are not row operations.
	if h, ok := a.top().(keyHandler); ok && h.HandleKey(ev) {
		return nil
	}

	// Row operations offered by the current view.
	for _, op := range a.top().Ops() {
		// Enter is handled by the Table's own SetSelectedFunc so it keeps
		// working with selection; capturing it here would double-fire.
		if op.Key == tcell.KeyEnter {
			continue
		}
		if op.Matches(ev) {
			a.runOp(op, a.rowsForOp())
			return nil
		}
	}

	// Swallow the keys we stole so they never reach Table's column scrolling.
	if ev.Key() == tcell.KeyRune && stolenKeys[ev.Rune()] {
		return nil
	}
	return ev
}

// handleGlobalKey handles the keys that work in every view.
func (a *App) handleGlobalKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyCtrlR:
		if t, ok := a.table(); ok {
			t.refreshNow(a.ctx)
			a.flash.show(flashInfo, "refreshing")
		}
		return true
	case tcell.KeyCtrlW:
		if t, ok := a.table(); ok {
			t.toggleWide()
		}
		return true
	case tcell.KeyCtrlA:
		if t, ok := a.table(); ok {
			t.markAllVisible()
		}
		return true
	case tcell.KeyCtrlBackslash:
		if t, ok := a.table(); ok {
			t.clearMarks()
		}
		return true
	case tcell.KeyEscape:
		// A filter is the innermost thing Escape should clear, before it
		// starts closing views.
		if t, ok := a.table(); ok && t.filterIn != "" {
			_ = t.setFilter("")
			a.flash.show(flashInfo, "filter cleared")
			return true
		}
		return false // let the Table's DoneFunc pop the view
	}

	if ev.Key() != tcell.KeyRune {
		return false
	}
	switch ev.Rune() {
	case ':':
		a.openPrompt(promptCommand)
		return true
	case '/':
		a.openPrompt(promptFilter)
		return true
	case '?':
		a.showHelp()
		return true
	case 'q':
		a.pop()
		return true
	case ' ':
		if t, ok := a.table(); ok {
			t.toggleMark()
		}
		return true
	}
	return false
}

// rowsForOp is the selection an operation applies to.
func (a *App) rowsForOp() []Row {
	if t, ok := a.table(); ok {
		return t.selection()
	}
	return nil
}

func (a *App) closeOverlay(name string) {
	a.pages.RemovePage(name)
	if v := a.top(); v != nil {
		a.app.SetFocus(v.Primitive())
	}
}
