package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// maxCellWidth truncates a runaway value — a description, a JSON blob —
// before it pushes every column after it off the screen.
const maxCellWidth = 48

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
	dynCols  []Column // set by resources whose columns come from fetched data

	sortCol  int // -1 when unsorted
	sortDesc bool

	interval time.Duration
	lastErr  string
	nextAt   time.Time
	fetching bool
}

func newTableView(a *App, res Resource, interval time.Duration) *tableView {
	t := tview.NewTable().
		SetSelectable(true, false).
		SetFixed(1, 0)
	// A framed table with the resource named on the top border, k9s-style:
	// the frame is what separates the list from the header block above it.
	t.SetBorder(true).SetBorderColor(colorBorder).SetTitleAlign(tview.AlignCenter)
	// A solid bar, not a tint: the selected row has to be unmistakable on a
	// screen where the delete key acts on it.
	t.SetSelectedStyle(tcell.StyleDefault.
		Background(colorSelected).
		Foreground(tcell.ColorBlack).
		Bold(true))

	v := &tableView{
		app:      a,
		res:      res,
		table:    t,
		marked:   map[string]bool{},
		sortCol:  -1,
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

// listing is one fetch's result. Columns travel with the rows so a resource
// that derives them from fetched data never has to store them itself.
type listing struct {
	columns []Column
	rows    []Row
}

func (v *tableView) Start(ctx context.Context) {
	v.fetching = true
	v.nextAt = time.Now().Add(v.interval)
	v.ref.start(ctx, v.interval,
		func(ctx context.Context) (any, error) {
			if dyn, ok := v.res.(dynamicColumns); ok {
				cols, rows, err := dyn.ListWithColumns(ctx, v.app.client)
				return listing{columns: cols, rows: rows}, err
			}
			rows, err := v.res.List(ctx, v.app.client)
			return listing{rows: rows}, err
		},
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
			got, _ := data.(listing)
			if got.columns != nil {
				v.dynCols = got.columns
			}
			v.rows = got.rows
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

// allColumns is the full column set in the order a row's cells are laid out.
func (v *tableView) allColumns() []Column {
	if v.dynCols != nil {
		return v.dynCols
	}
	return v.res.Columns()
}

// columns returns the columns to draw, dropping wide ones unless asked for.
func (v *tableView) columns() []Column {
	all := v.allColumns()
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
	all := v.allColumns()
	for i, c := range cols {
		heading := c.Name
		if i == v.sortCol && v.sortCol >= 0 {
			heading += sortArrow(v.sortDesc)
		}
		cell := tview.NewTableCell(heading).
			SetTextColor(colorTitle).
			SetAttributes(tcell.AttrBold).
			SetSelectable(false).
			SetMaxWidth(maxCellWidth)
		if i == len(cols)-1 {
			cell.SetExpansion(1)
		}
		v.table.SetCell(0, i, cell)
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
			prefix = "› "
		}
		dim := v.isInert(r)
		for ci, src := range index {
			text := ""
			if src < len(r.Cells) {
				text = r.Cells[src]
			}
			// No expansion: columns pack left with a single space between
			// them, as k9s does. Spreading them to fill the width puts
			// yards of whitespace between related values.
			cell := tview.NewTableCell(prefix + text).SetMaxWidth(maxCellWidth)
			// Only the last column stretches. That keeps the others packed
			// left as k9s has them, while letting the selection bar run to
			// the right edge instead of stopping at the last character.
			if ci == len(index)-1 {
				cell.SetExpansion(1)
			}
			switch {
			case dim:
				// Nothing more will happen to this row, so it recedes.
				cell.SetTextColor(colorDimmed)
			case ci == 0:
				// The identifier is what the eye scans down, so it carries
				// the accent even when the rest of the row is plain.
				cell.SetTextColor(colorAccent)
			default:
				if c := statusColor(strings.TrimSpace(text)); c != tcell.ColorDefault {
					cell.SetTextColor(c)
				}
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
			// A first load, or one whose selected row has gone, starts at the
			// top. Resetting the offset is not redundant: the table is drawn
			// once while still empty, which leaves tview tracking the end, and
			// it then shows the *last* page with the cursor invisibly on row
			// one until some key nudges it.
			restored = 1
			v.table.ScrollToBeginning()
		}
		v.table.Select(restored, 0)
	}
	v.setTitle()
	v.app.drawHeader()
}

// isInert reports whether a row represents something spent — deleted,
// destroyed, abandoned — which is drawn greyed out.
func (v *tableView) isInert(r Row) bool {
	for _, cell := range r.Cells {
		if inertStatuses[strings.TrimSpace(cell)] {
			return true
		}
	}
	return false
}

// setTitle names the resource on the border, with its live counts and any
// active filter, in the shape k9s labels a table: pods(all)[31] </po>.
//
// Each piece gets its own colour — the narrowing argument, the row count, the
// filter — so the title stays one glance rather than a sentence to read.
func (v *tableView) setTitle() {
	title := fmt.Sprintf(" [%s::b]%s", tagAccent, v.res.Kind())
	if arg := titleArg(v.res); arg != "" {
		title += fmt.Sprintf("[%s::-]([%s::b]%s[%s::-])", tagAccent, tagHighlight, arg, tagAccent)
	}
	title += fmt.Sprintf("[%s::-][[%s::b]%d[%s::-]] ", tagAccent, tagCounter, len(v.visible), tagAccent)
	if v.filterIn != "" {
		title += fmt.Sprintf("<[%s::b]/%s[%s::-]> ", tagFilter, v.filterIn, tagAccent)
	}
	v.table.SetTitle(title)
}

// titleArg is whatever narrows a resource — the blueprint, the entity —
// taken from its id, which already carries exactly that.
func titleArg(res Resource) string {
	rest := strings.TrimPrefix(res.ID(), res.Kind())
	return strings.Join(strings.Fields(rest), " ")
}

// sortArrow marks the sorted column in its heading.
func sortArrow(desc bool) string {
	if desc {
		return "↓"
	}
	return "↑"
}

func (v *tableView) counts() (shown, total, marked int) {
	return len(v.visible), len(v.rows), len(v.marked)
}
