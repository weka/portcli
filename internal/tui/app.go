package tui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
)

// Options configures a TUI session.
type Options struct {
	Version string
	Config  *config.Config
	// Refresh overrides the remembered interval when non-zero.
	Refresh time.Duration
	// View overrides the remembered starting view when non-empty.
	View string
}

// defaultInterval is the fallback poll interval. Individual resources may ask
// for something different via intervalFor.
const defaultInterval = 30 * time.Second

// App owns the screen for the lifetime of a session.
type App struct {
	app    *tview.Application
	client *client.Client
	opts   Options
	state  State

	pages   *tview.Pages
	ctxInfo *tview.TextView
	hints   *tview.TextView
	logo    *tview.TextView
	crumbs  *tview.TextView
	prompt  *tview.InputField
	bar     *tview.Pages // swaps between crumbs and prompt
	body    *tview.Pages
	flash   *flash

	stack   []View
	ctx     context.Context
	caches  map[string][]string
	promptM promptMode
}

// Run takes over the terminal until the user quits.
//
// It returns only after the screen is restored, so a caller's os.Exit can
// never fire while tcell still holds raw mode.
func Run(ctx context.Context, c *client.Client, opts Options) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("cannot open a terminal screen (is TERM set?): %w", err)
	}
	if err := screen.Init(); err != nil {
		return fmt.Errorf("cannot initialise the terminal (TERM=%q): %w", termEnv(), err)
	}
	return run(ctx, c, opts, screen)
}

// run drives the UI on an already-initialised screen. Separate from Run so a
// test can supply a tcell simulation screen.
func run(ctx context.Context, c *client.Client, opts Options, screen tcell.Screen) error {
	// tview recovers panics on its own goroutine but not on one the TUI
	// spawned, and a panic with the screen still live leaves the terminal in
	// raw mode with no prompt.
	defer func() {
		screen.Fini()
		if p := recover(); p != nil {
			panic(p)
		}
	}()

	// Anything a dependency logs would be drawn over the screen.
	log.SetOutput(io.Discard)

	a := newApp(ctx, c, opts)
	a.app.SetScreen(screen)

	// An external SIGTERM cancels ctx; tear the UI down rather than orphaning it.
	go func() {
		<-ctx.Done()
		a.app.Stop()
	}()

	if runErr := a.app.Run(); runErr != nil {
		return runErr
	}
	a.saveState()
	return nil
}

func newApp(ctx context.Context, c *client.Client, opts Options) *App {
	a := &App{
		app:    tview.NewApplication(),
		client: c,
		opts:   opts,
		state:  LoadState(),
		ctx:    ctx,
		caches: map[string][]string{},
	}
	if opts.View != "" {
		a.state.LastView = opts.View
	}

	a.ctxInfo = tview.NewTextView().SetDynamicColors(true)
	a.hints = tview.NewTextView().SetDynamicColors(true)
	a.logo = tview.NewTextView().SetDynamicColors(true).SetTextAlign(tview.AlignRight)
	a.logo.SetText("[dodgerblue::b]portcli")

	header := tview.NewFlex().
		AddItem(a.ctxInfo, 0, 2, false).
		AddItem(a.hints, 0, 2, false).
		AddItem(a.logo, 12, 0, false)

	a.crumbs = tview.NewTextView().SetDynamicColors(true)
	a.prompt = tview.NewInputField()
	a.prompt.SetFieldBackgroundColor(tcell.ColorDefault)
	// The prompt replaces the breadcrumb line rather than floating over the
	// table, so entering a command does not reflow the layout.
	a.bar = tview.NewPages().
		AddPage("crumbs", a.crumbs, true, true).
		AddPage("prompt", a.prompt, true, false)

	a.body = tview.NewPages()
	a.flash = newFlash(a.app)

	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 7, 0, false).
		AddItem(a.bar, 1, 0, false).
		AddItem(a.body, 0, 1, true).
		AddItem(a.flash.view, 1, 0, false)

	a.pages = tview.NewPages().AddPage("main", main, true, true)
	a.app.SetRoot(a.pages, true)
	a.app.SetInputCapture(a.globalCapture)
	a.configurePrompt()

	a.openInitialView()
	return a
}

// openInitialView restores the remembered view, falling back to blueprints.
// A remembered view that no longer resolves — a blueprint since deleted — is a
// flash, never a fatal: a stale state file must not make the TUI unopenable.
func (a *App) openInitialView() {
	if err := a.open(a.state.LastView); err == nil {
		return
	} else if a.state.LastView != defaultView {
		a.flash.show(flashWarn, "could not reopen %q, showing %s", a.state.LastView, defaultView)
	}
	if err := a.open(defaultView); err != nil {
		a.fatal(err)
	}
}

// open parses palette input and pushes the resulting view.
func (a *App) open(input string) error {
	cmd, ok := ParseCommand(input)
	if !ok {
		return fmt.Errorf("nothing to open")
	}
	res, err := Resolve(cmd.Kind, cmd.Args)
	if err != nil {
		return err
	}
	if f, ok := res.(filterable); ok && cmd.Filter != "" {
		if err := f.SetServerFilter(cmd.Filter); err != nil {
			return err
		}
	}
	a.push(newTableView(a, res, a.intervalFor(res.Kind())))
	return nil
}

// filterable is implemented by resources that can push a filter to the API
// rather than filtering client-side.
type filterable interface {
	SetServerFilter(filter string) error
}

// intervalFor picks a poll interval per resource. Definitions change rarely;
// runs are the volatile thing worth watching closely.
func (a *App) intervalFor(kind string) time.Duration {
	if a.opts.Refresh > 0 {
		return a.opts.Refresh
	}
	base := a.state.RefreshInterval(defaultInterval)
	switch kind {
	case "blueprints":
		return 60 * time.Second
	case "actions":
		return 120 * time.Second
	case "runs":
		return 10 * time.Second
	default:
		return base
	}
}

// push shows a view and makes it the only one refreshing.
func (a *App) push(v View) {
	if len(a.stack) > 0 {
		a.top().Stop()
	}
	a.stack = append(a.stack, v)
	if !a.body.HasPage(v.ID()) {
		a.body.AddPage(v.ID(), v.Primitive(), true, false)
	}
	a.body.SwitchToPage(v.ID())
	a.app.SetFocus(v.Primitive())
	v.Start(a.ctx)
	a.drawHeader()
	a.drawCrumbs()
}

// pop returns to the previous view, quitting when the last one is closed —
// which is what makes q and Esc feel like "back" everywhere.
func (a *App) pop() {
	if len(a.stack) <= 1 {
		a.quit()
		return
	}
	leaving := a.top()
	leaving.Stop()
	a.body.RemovePage(leaving.ID())
	a.stack = a.stack[:len(a.stack)-1]

	v := a.top()
	a.body.SwitchToPage(v.ID())
	a.app.SetFocus(v.Primitive())
	v.Start(a.ctx)
	a.drawHeader()
	a.drawCrumbs()
}

// table returns the current view as a table, if it is one. Most global keys
// and palette commands only mean something over a table, and naming that once
// keeps the type assertion out of every one of them.
func (a *App) table() (*tableView, bool) {
	t, ok := a.top().(*tableView)
	return t, ok
}

func (a *App) top() View {
	if len(a.stack) == 0 {
		return nil
	}
	return a.stack[len(a.stack)-1]
}

func (a *App) quit() {
	a.saveState()
	a.app.Stop()
}

// saveState remembers the deepest view that can be reopened from a command
// string. Failures are reported, never fatal.
func (a *App) saveState() {
	for i := len(a.stack) - 1; i >= 0; i-- {
		if r, ok := a.stack[i].(*tableView); ok {
			a.state.LastView = r.res.ID()
			break
		}
	}
	_ = a.state.Save()
}

func (a *App) drawCrumbs() {
	var parts []string
	for i, v := range a.stack {
		if i == len(a.stack)-1 {
			parts = append(parts, "[dodgerblue::b]<"+v.Title()+">[-::-]")
			continue
		}
		parts = append(parts, "[dimgray]<"+v.Title()+">[-]")
	}
	a.crumbs.SetText(strings.Join(parts, " "))
}

func (a *App) drawHeader() {
	info := headerInfo{
		BaseURL:    a.opts.Config.BaseURL,
		ClientID:   a.opts.Config.ClientID,
		AuthSource: authSource(),
		Version:    a.opts.Version,
	}
	if v := a.top(); v != nil {
		info.Resource = v.ID()
		if t, ok := v.(*tableView); ok {
			info.Interval = t.interval
			info.NextIn = time.Until(t.nextAt)
			info.Refreshing = t.fetching
			info.RefreshFail = t.lastErr
			info.Shown, info.Total, info.Marked = t.counts()
		}
		a.hints.SetText(strings.Join(hintLines(v.Ops()), "\n"))
	}
	a.ctxInfo.SetText(strings.Join(info.headerLines(), "\n"))
}

// runOp executes a row operation, routing anything destructive through a
// confirmation first.
func (a *App) runOp(op Op, rows []Row) {
	if len(rows) == 0 {
		a.flash.show(flashWarn, "nothing selected")
		return
	}
	if op.Dangerous {
		a.confirm(op, rows)
		return
	}
	if err := op.Run(a, rows); err != nil {
		a.showError(err)
	}
}

// showError uses a modal rather than the flash line: an operation the user
// asked for failing is worth blocking on, unlike a background refresh.
func (a *App) showError(err error) {
	modal := tview.NewModal().
		SetText(err.Error()).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) {
			a.pages.RemovePage("error")
			a.app.SetFocus(a.top().Primitive())
		})
	a.pages.AddPage("error", modal, true, true)
}

// fatal replaces the UI. Used when nothing further can work — broken
// credentials, or no view at all — so a flash would be dishonest.
func (a *App) fatal(err error) {
	text := fmt.Sprintf("[indianred::b]portcli cannot continue[-::-]\n\n%v\n\n[dimgray]Base URL: %s\nAuth:     %s\n\nPress q to quit.",
		err, a.opts.Config.BaseURL, authSource())
	view := tview.NewTextView().SetDynamicColors(true).SetText(text)
	view.SetDoneFunc(func(tcell.Key) { a.app.Stop() })
	a.pages.AddPage("fatal", view, true, true)
	a.app.SetFocus(view)
}

func termEnv() string { return os.Getenv("TERM") }
