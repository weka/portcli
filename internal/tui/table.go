package tui

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// tableView renders any Resource. One instance per stack entry, so returning
// to a view restores its scroll position, filter and marks.
type tableView struct {
	app *App
	res Resource

	table *tview.Table
	ref   refresher

	rows     []Row // everything the last fetch returned
	visible  []Row // rows passing the filter, in display order
	filter   *regexp.Regexp
	filterIn string
	marked   map[string]bool
	wide     bool

	interval time.Duration
	lastErr  string
	nextAt   time.Time
	fetching bool
}

func newTableView(a *App, res Resource, interval time.Duration) *tableView {
	t := tview.NewTable().
		SetSelectable(true, false).
		SetFixed(1, 0)
	t.SetBorder(false)
	t.SetSelectedStyle(tcell.StyleDefault.Background(colorSelected).Bold(true))

	v := &tableView{
		app:      a,
		res:      res,
		table:    t,
		marked:   map[string]bool{},
		interval: interval,
	}
	v.ref.app = a.app

	// Enter is wired through the table rather than the global capture so it
	// keeps working with the table's own selection handling.
	t.SetSelectedFunc(func(row, _ int) {
		for _, op := range res.Ops() {
			if op.Key == tcell.KeyEnter {
				a.runOp(op, v.selection())
				return
			}
		}
	})
	// Escape reaches a Table through SetDoneFunc, not InputCapture.
	t.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			a.pop()
		}
	})
	return v
}

func (v *tableView) ID() string                 { return v.res.ID() }
func (v *tableView) Title() string              { return v.res.Title() }
func (v *tableView) Primitive() tview.Primitive { return v.table }
func (v *tableView) Ops() []Op                  { return v.res.Ops() }
func (v *tableView) Stop()                      { v.ref.stop() }

func (v *tableView) Start(ctx context.Context) {
	v.fetching = true
	v.nextAt = time.Now().Add(v.interval)
	v.ref.start(ctx, v.interval,
		func(ctx context.Context) (any, error) { return v.res.List(ctx, v.app.client) },
		func(data any, err error) {
			v.fetching = false
			v.nextAt = time.Now().Add(v.interval)
			if err != nil {
				// Stale rows are more useful than a blank table, so the
				// failure is reported without discarding what is on screen.
				v.lastErr = err.Error()
				v.app.flash.show(flashWarn, "refresh failed: %v", err)
				v.app.drawHeader()
				return
			}
			v.lastErr = ""
			rows, _ := data.([]Row)
			v.rows = rows
			v.render()
		})
}

// refreshNow forces a cycle by restarting the loop, which fetches immediately.
func (v *tableView) refreshNow(ctx context.Context) {
	v.Start(ctx)
}

// setFilter applies a client-side regex over the visible cells. An invalid
// pattern is reported rather than silently matching nothing.
func (v *tableView) setFilter(pattern string) error {
	v.filterIn = pattern
	if pattern == "" {
		v.filter = nil
		v.render()
		return nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return err
	}
	v.filter = re
	v.render()
	return nil
}

func (v *tableView) toggleWide() {
	v.wide = !v.wide
	v.render()
}

// selection returns the rows an operation applies to: every marked row if any
// are marked, otherwise the row under the cursor.
func (v *tableView) selection() []Row {
	if len(v.marked) > 0 {
		var out []Row
		for _, r := range v.visible {
			if v.marked[r.ID] {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if r, ok := v.current(); ok {
		return []Row{r}
	}
	return nil
}

func (v *tableView) current() (Row, bool) {
	row, _ := v.table.GetSelection()
	idx := row - 1 // row 0 is the header
	if idx < 0 || idx >= len(v.visible) {
		return Row{}, false
	}
	return v.visible[idx], true
}

func (v *tableView) toggleMark() {
	if r, ok := v.current(); ok {
		if v.marked[r.ID] {
			delete(v.marked, r.ID)
		} else {
			v.marked[r.ID] = true
		}
		v.render()
	}
}

func (v *tableView) markAllVisible() {
	for _, r := range v.visible {
		v.marked[r.ID] = true
	}
	v.render()
}

func (v *tableView) clearMarks() {
	v.marked = map[string]bool{}
	v.render()
}

// columns returns the columns to draw, dropping wide ones unless asked for.
func (v *tableView) columns() []Column {
	all := v.res.Columns()
	if v.wide {
		return all
	}
	out := make([]Column, 0, len(all))
	for _, c := range all {
		if !c.Wide {
			out = append(out, c)
		}
	}
	return out
}

func (v *tableView) matches(r Row) bool {
	if v.filter == nil {
		return true
	}
	for _, cell := range r.Cells {
		if v.filter.MatchString(cell) {
			return true
		}
	}
	return v.filter.MatchString(r.ID)
}

// render redraws the table, preserving the selected row by ID so a refresh
// that reorders rows does not move the cursor onto a different entity.
func (v *tableView) render() {
	selectedID := ""
	if r, ok := v.current(); ok {
		selectedID = r.ID
	}

	v.visible = v.visible[:0]
	for _, r := range v.rows {
		if v.matches(r) {
			v.visible = append(v.visible, r)
		}
	}

	v.table.Clear()
	cols := v.columns()
	all := v.res.Columns()
	for i, c := range cols {
		v.table.SetCell(0, i, tview.NewTableCell(c.Name).
			SetTextColor(colorTitle).
			SetAttributes(tcell.AttrBold).
			SetSelectable(false).
			SetExpansion(1))
	}

	// Cells arrive in the resource's full column order, so dropping wide
	// columns from the display means indexing by name, not position.
	index := make([]int, 0, len(cols))
	for _, c := range cols {
		for j, a := range all {
			if a.Name == c.Name {
				index = append(index, j)
				break
			}
		}
	}

	restored := 0
	for i, r := range v.visible {
		prefix := ""
		if v.marked[r.ID] {
			prefix = "[dodgerblue]›[-] "
		}
		for ci, src := range index {
			text := ""
			if src < len(r.Cells) {
				text = r.Cells[src]
			}
			cell := tview.NewTableCell(prefix + text).SetExpansion(1)
			if ci == 0 && v.marked[r.ID] {
				cell.SetTextColor(colorTitle)
			} else if c := statusColor(strings.TrimSpace(text)); c != tcell.ColorDefault {
				cell.SetTextColor(c)
			}
			v.table.SetCell(i+1, ci, cell)
			prefix = ""
		}
		if r.ID == selectedID {
			restored = i + 1
		}
	}

	if len(v.visible) > 0 {
		if restored == 0 {
			restored = 1
		}
		v.table.Select(restored, 0)
	}
	v.app.drawHeader()
}

func (v *tableView) counts() (shown, total, marked int) {
	return len(v.visible), len(v.rows), len(v.marked)
}
