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

// configurePrompt wires the one InputField that serves both ":" and "/".
func (a *App) configurePrompt() {
	a.prompt.SetAutocompleteFunc(func(current string) []string {
		if a.promptM != promptCommand {
			return nil
		}
		return Candidates(current, a.caches)
	})

	a.prompt.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			a.submitPrompt(a.prompt.GetText())
		case tcell.KeyEscape:
			a.closePrompt()
		}
	})
}

func (a *App) openPrompt(mode promptMode) {
	a.promptM = mode
	label, seed := "> ", ""
	if mode == promptFilter {
		label = "/"
		// Seed with the active filter so "/" is an edit, not a retype.
		if t, ok := a.table(); ok {
			seed = t.filterIn
		}
	}
	a.prompt.SetLabel(label).SetText(seed)
	a.bar.SwitchToPage("prompt")
	a.app.SetFocus(a.prompt)
}

func (a *App) closePrompt() {
	a.bar.SwitchToPage("crumbs")
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
