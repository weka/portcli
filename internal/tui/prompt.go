package tui

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

// filterView is a view that can narrow what it shows. Tables filter rows; the
// log pane filters lines.
type filterView interface {
	setFilter(pattern string) error
}

type promptMode int

const (
	promptCommand promptMode = iota // ":" — open a resource
	promptFilter                    // "/" — narrow the current table
)

// Heights of the bar row: the breadcrumb trail is one line, the prompt is one
// line inside a border.
const (
	crumbHeight  = 1
	promptHeight = 3
)

// configurePrompt wires the one InputField that serves both ":" and "/".
func (a *App) configurePrompt() {
	// Styles first, and not for tidiness: SetAutocompleteFunc runs the callback
	// straight away, and the completion list is built with whatever styles are
	// set at the moment it is first created — a later SetAutocompleteStyles is
	// silently ignored for the life of that list.
	//
	// They are worth setting because tview draws the list outside the field's
	// rect and clips nothing, so it lands on top of the table. A filled slate
	// block reads as a panel floating over it; tview's default read as a
	// bright bar across a border, and a background matching the screen's would
	// leave the entries looking embedded in the border they cover.
	a.prompt.SetAutocompleteStyles(
		colorPanel,
		tcell.StyleDefault.Background(colorPanel).Foreground(colorKey),
		tcell.StyleDefault.Background(colorKey).Foreground(tcell.ColorBlack).Bold(true),
	)
	a.prompt.SetAutocompleteFunc(func(current string) []string {
		if a.promptM != promptCommand {
			return nil
		}
		return Candidates(current, a.caches)
	})

	// Filtering is live, as it is in k9s: the table narrows and its title
	// count moves as you type, so a pattern is judged by what it leaves on
	// screen rather than by guessing and pressing Enter. A half-typed pattern
	// is often not a valid regex — "a(" — so a compile failure just leaves the
	// last good filter in place until the next keystroke; Enter is what
	// reports it.
	a.prompt.SetChangedFunc(func(text string) {
		if a.promptM != promptFilter {
			return
		}
		if f, ok := a.top().(filterView); ok {
			_ = f.setFilter(text)
		}
	})

	a.prompt.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			a.submitPrompt(a.prompt.GetText())
		case tcell.KeyEscape:
			a.cancelPrompt()
		}
	})
}

func (a *App) openPrompt(mode promptMode) {
	a.promptM = mode
	// The border colour is the mode: aqua asks for a command, green narrows
	// what is already on screen. It is the same green the active filter wears
	// in the table's title.
	label, seed, color := "> ", "", colorCommand
	if mode == promptFilter {
		label, color = "/", colorFilter
		// Seed with the active filter so "/" is an edit, not a retype.
		if t, ok := a.table(); ok {
			seed = t.filterIn
		}
	}
	a.promptSeed = seed
	a.prompt.SetBorderColor(color)
	a.prompt.SetLabelColor(color)
	// SetText fires the change handler, which reads promptM — already set
	// above, so a seeded filter re-applies as a filter rather than being
	// dispatched by whatever mode the last prompt was in.
	a.prompt.SetLabel(label).SetText(seed)

	a.showBar(true)
	a.app.SetFocus(a.prompt)
}

// showBar puts either the breadcrumb trail or the prompt on the bar row. They
// differ in height as well as in content — the prompt is boxed — so one call
// sets both, rather than leaving a page switch and a resize to be kept in step
// at each call site.
func (a *App) showBar(prompting bool) {
	page, height := "crumbs", crumbHeight
	if prompting {
		page, height = "prompt", promptHeight
	}
	a.main.ResizeItem(a.bar, height, 0)
	a.bar.SwitchToPage(page)
}

// cancelPrompt abandons the prompt. Live filtering means Escape has something
// to undo: whatever was typed is already applied, so the filter in force when
// the prompt opened is put back.
func (a *App) cancelPrompt() {
	if a.promptM == promptFilter {
		if f, ok := a.top().(filterView); ok {
			_ = f.setFilter(a.promptSeed)
		}
	}
	a.closePrompt()
}

func (a *App) closePrompt() {
	a.showBar(false)
	if v := a.top(); v != nil {
		a.app.SetFocus(v.Primitive())
	}
}

func (a *App) submitPrompt(text string) {
	mode := a.promptM
	a.closePrompt()

	if mode == promptFilter {
		// Tables and the log pane both filter, over different things.
		f, ok := a.top().(filterView)
		if !ok {
			a.flash.show(flashWarn, "this view cannot be filtered")
			return
		}
		if err := f.setFilter(text); err != nil {
			a.flash.show(flashError, "bad filter pattern: %v", err)
			return
		}
		if text == "" {
			a.flash.show(flashInfo, "filter cleared")
			return
		}
		if t, ok := a.table(); ok {
			shown, total, _ := t.counts()
			if shown == 0 {
				a.flash.show(flashWarn, "no rows match %q (of %d)", text, total)
			} else {
				a.flash.show(flashInfo, "%d of %d rows match %q", shown, total, text)
			}
			return
		}
		a.flash.show(flashInfo, "filtering on %q", text)
		return
	}

	if text == "" {
		return
	}
	if err := a.runCommand(text); err != nil {
		a.flash.show(flashError, "%v", err)
	}
}

// runCommand handles palette input: a few built-ins, otherwise a resource to
// open.
func (a *App) runCommand(text string) error {
	cmd, ok := ParseCommand(text)
	if !ok {
		return fmt.Errorf("nothing to run")
	}

	switch cmd.Kind {
	case "q", "quit", "exit":
		a.quit()
		return nil
	case "help", "h", "?":
		a.showHelp()
		return nil
	case "errors":
		a.showErrorHistory()
		return nil
	case "refresh":
		return a.setRefresh(cmd.Args)
	case "cols", "columns":
		return a.setColumns(cmd.Args)
	}
	return a.open(text)
}

// setColumns overrides the columns a resource chose for itself.
func (a *App) setColumns(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: :cols <property>[,<property>...]")
	}
	t, ok := a.table()
	if !ok {
		return fmt.Errorf("this view has no columns")
	}
	setter, ok := t.res.(interface{ SetColumns([]string) })
	if !ok {
		return fmt.Errorf("%s does not take column overrides", t.res.Kind())
	}

	var cols []string
	for _, group := range args {
		for _, name := range strings.Split(group, ",") {
			if name = strings.TrimSpace(name); name != "" {
				cols = append(cols, name)
			}
		}
	}
	setter.SetColumns(cols)
	t.refreshNow(a.ctx)
	a.flash.show(flashInfo, "columns: %s", strings.Join(cols, ", "))
	return nil
}

// setRefresh changes the current view's poll interval for this session.
func (a *App) setRefresh(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: :refresh <duration>, e.g. :refresh 10s")
	}
	d, err := parsePositiveDuration(args[0])
	if err != nil {
		return err
	}
	t, ok := a.table()
	if !ok {
		return fmt.Errorf("this view does not refresh")
	}
	t.interval = d
	a.state.Refresh = args[0]
	t.refreshNow(a.ctx)
	a.flash.show(flashInfo, "refreshing every %s", d)
	return nil
}
