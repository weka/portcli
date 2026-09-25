package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// parsePositiveDuration parses a duration and rejects the non-positive ones,
// which would make a ticker panic.
func parsePositiveDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("not a duration: %q (try 10s, 1m)", s)
	}
	if d <= 0 {
		return 0, fmt.Errorf("duration must be positive, got %s", d)
	}
	return d, nil
}

// showHelp renders the key map, generated from the live bindings rather than
// written out separately, so it cannot drift from what the keys actually do.
func (a *App) showHelp() {
	t := tview.NewTable().SetBorders(false)
	t.SetTitle(" help ").SetBorder(true).SetBorderColor(colorBorder)

	row := 0
	section := func(name string) {
		t.SetCell(row, 0, tview.NewTableCell("").SetSelectable(false))
		row++
		t.SetCell(row, 0, tview.NewTableCell(name).
			SetTextColor(colorTitle).SetAttributes(tcell.AttrBold).SetSelectable(false))
		row++
	}
	entry := func(key, desc string) {
		t.SetCell(row, 0, tview.NewTableCell("  "+key).SetTextColor(colorLabel).SetSelectable(false))
		t.SetCell(row, 1, tview.NewTableCell(desc).SetSelectable(false))
		row++
	}

	if v := a.top(); v != nil {
		section(v.Title())
		for _, op := range v.Ops() {
			desc := op.Name
			if op.Dangerous {
				desc += " (confirms first)"
			}
			entry(op.Label(), desc)
		}
	}

	section("Navigate")
	for _, e := range [][2]string{
		{"j / k", "down / up"},
		{"g / G", "first / last row"},
		{"ctrl-f / ctrl-b", "page down / up"},
		{"← / →", "scroll columns"},
		{"enter", "drill into the selected row"},
		{"esc", "clear the filter, else go back"},
		{"q", "back, or quit from the top"},
	} {
		entry(e[0], e[1])
	}

	section("Everywhere")
	for _, e := range [][2]string{
		{":", "command palette — " + strings.Join(Kinds(), ", ")},
		{"/", "filter the rows on screen (regex)"},
		{"space", "mark a row; operations then apply to all marks"},
		{"ctrl-a", "mark every visible row"},
		{"ctrl-\\", "clear marks"},
		{"ctrl-r", "refresh now"},
		{"ctrl-w", "show or hide wide columns"},
		{"?", "this help"},
		{"ctrl-c", "quit immediately"},
	} {
		entry(e[0], e[1])
	}

	section("Palette commands")
	for _, e := range [][2]string{
		{":<resource>", "open it, e.g. :entities my-blueprint"},
		{":refresh 10s", "change this view's poll interval"},
		{":errors", "recent messages, including refreshes that failed"},
		{":q", "quit"},
	} {
		entry(e[0], e[1])
	}

	t.SetDoneFunc(func(tcell.Key) { a.closeOverlay("help") })
	a.pages.AddPage("help", centered(t, 78, 0), true, true)
	a.app.SetFocus(t)
}

// showErrorHistory lists recent flashes. A TUI takes the scrollback away, so
// without this a refresh failure from ten minutes ago is simply gone.
func (a *App) showErrorHistory() {
	v := tview.NewTextView().SetDynamicColors(true).SetScrollable(true)
	v.SetTitle(" recent messages ").SetBorder(true).SetBorderColor(colorBorder)

	if len(a.flash.history) == 0 {
		v.SetText("[dimgray]nothing yet")
	} else {
		var b strings.Builder
		for i := len(a.flash.history) - 1; i >= 0; i-- {
			e := a.flash.history[i]
			fmt.Fprintf(&b, "[dimgray]%s[-] %s%s[-]\n",
				e.At.Format("15:04:05"), e.Level.tag(), e.Message)
		}
		v.SetText(b.String())
	}

	v.SetDoneFunc(func(tcell.Key) { a.closeOverlay("errors") })
	a.pages.AddPage("errors", centered(v, 100, 0), true, true)
	a.app.SetFocus(v)
}

// centered wraps a primitive in flexes that keep it to at most width x height,
// centred. A zero dimension means "fill".
func centered(p tview.Primitive, width, height int) tview.Primitive {
	col := tview.NewFlex().SetDirection(tview.FlexRow)
	if height > 0 {
		col.AddItem(tview.NewBox(), 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(tview.NewBox(), 0, 1, false)
	} else {
		col.AddItem(p, 0, 1, true)
	}

	rowFlex := tview.NewFlex()
	if width > 0 {
		rowFlex.AddItem(tview.NewBox(), 0, 1, false).
			AddItem(col, width, 0, true).
			AddItem(tview.NewBox(), 0, 1, false)
	} else {
		rowFlex.AddItem(col, 0, 1, true)
	}
	return rowFlex
}
