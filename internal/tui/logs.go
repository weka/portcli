package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
	"github.com/weka/portcli/internal/upsert"
)

const (
	// logsInterval polls quickly: logs are what you watch a running action
	// through.
	logsInterval = 2 * time.Second
	// logsMaxLines bounds memory on a long run.
	logsMaxLines = 5000
)

// logsView tails one run's logs.
//
// It fetches from the number of lines already seen rather than refetching the
// whole log each tick, so a long-running action costs a few lines per poll
// instead of everything every two seconds.
type logsView struct {
	app   *App
	runID string
	view  *tview.TextView
	ref   refresher

	lines  []client.RunLog
	filter *regexp.Regexp
	follow bool
	// sawAny distinguishes "no output yet" from "this run does not exist",
	// which the API reports identically.
	sawAny bool
	failed string
}

func newLogsView(a *App, runID string) *logsView {
	v := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetWrap(false)
	v.SetTitle(fmt.Sprintf(" logs: %s ", runID)).SetBorder(true).SetBorderColor(colorBorder)

	l := &logsView{app: a, runID: runID, view: v, follow: true}
	l.ref.app = a.app
	v.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			a.pop()
		}
	})
	return l
}

func (v *logsView) ID() string                 { return "logs/" + v.runID }
func (v *logsView) Title() string              { return "logs(" + v.runID + ")" }
func (v *logsView) Primitive() tview.Primitive { return v.view }
func (v *logsView) Stop()                      { v.ref.stop() }

func (v *logsView) Ops() []Op {
	return []Op{
		{Rune: 'f', Name: "Follow on/off", Run: func(a *App, _ []Row) error {
			v.follow = !v.follow
			state := "off"
			if v.follow {
				state = "on"
			}
			a.flash.show(flashInfo, "follow %s", state)
			v.render()
			return nil
		}},
		{Rune: 'y', Name: "Copy to clipboard", Run: func(a *App, _ []Row) error {
			return copyToClipboard(a, v.plain())
		}},
	}
}

func (v *logsView) Start(ctx context.Context) {
	v.ref.start(ctx, logsInterval,
		func(ctx context.Context) (any, error) {
			return v.app.client.GetRunLogsFrom(ctx, v.runID, len(v.lines), 0)
		},
		func(data any, err error) {
			if err != nil {
				v.failed = upsertAwareError(err).Error()
				v.render()
				return
			}
			v.failed = ""
			fresh, _ := data.([]client.RunLog)
			if len(fresh) > 0 {
				v.sawAny = true
				v.lines = append(v.lines, fresh...)
				if len(v.lines) > logsMaxLines {
					v.lines = v.lines[len(v.lines)-logsMaxLines:]
				}
			}
			v.render()
		})
}

// HandleKey gives the log pane its own filter, separate from the table "/".
func (v *logsView) HandleKey(ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyRune && ev.Rune() == '/' {
		v.app.openPrompt(promptFilter)
		return true
	}
	return false
}

// setFilter narrows the displayed lines.
func (v *logsView) setFilter(pattern string) error {
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

func (v *logsView) plain() string {
	var b strings.Builder
	for _, l := range v.lines {
		fmt.Fprintf(&b, "[%s] %s\n", portfmt.DateTime(l.CreatedAt), l.Message)
	}
	return b.String()
}

func (v *logsView) render() {
	var b strings.Builder
	shown := 0
	for _, l := range v.lines {
		if v.filter != nil && !v.filter.MatchString(l.Message) {
			continue
		}
		shown++
		fmt.Fprintf(&b, "[dimgray]%s[-] %s\n",
			portfmt.DateTime(l.CreatedAt), tview.Escape(l.Message))
	}

	switch {
	case v.failed != "":
		fmt.Fprintf(&b, "\n[indianred]%s[-]\n", v.failed)
	case !v.sawAny:
		// An empty list is genuinely ambiguous: the endpoint answers 200 with
		// no entries for a run that does not exist, exactly as it does for a
		// run that has produced nothing yet. Saying so beats implying either.
		b.WriteString("[dimgray]no logs yet — the run may have produced no output, or may not exist[-]\n")
	case shown == 0 && v.filter != nil:
		fmt.Fprintf(&b, "[orange]no lines match the filter (%d hidden)[-]\n", len(v.lines))
	}

	v.view.SetText(b.String())
	if v.follow {
		v.view.ScrollToEnd()
	}
	title := fmt.Sprintf(" logs: %s — %d lines ", v.runID, len(v.lines))
	if !v.follow {
		title += "[paused] "
	}
	v.view.SetTitle(title)
}

// upsertAwareError annotates a 404 on a run lookup. Port discards the run
// record for UPSERT_ENTITY actions the moment they finish, so those ids never
// resolve whether the action worked or not; unannotated, the 404 reads as a
// lost run.
func upsertAwareError(err error) error {
	return upsert.ExplainMissingRun(err)
}
