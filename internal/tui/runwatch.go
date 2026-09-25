package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

const (
	// watchInterval while a run is in progress.
	watchInterval = 2 * time.Second
	// settleTicks keeps polling briefly after a terminal status, because logs
	// can still land after the status flips.
	settleTicks = 3
)

// runWatchView follows one run: status above, logs below.
type runWatchView struct {
	app      *App
	runID    string
	actionID string

	card *tview.TextView
	logs *logsView
	flex *tview.Flex
	ref  refresher

	run      *client.ActionRun
	lastErr  string
	settling int
	stopped  bool
}

func newRunWatchView(a *App, runID, actionID string) *runWatchView {
	card := tview.NewTextView().SetDynamicColors(true)
	card.SetTitle(fmt.Sprintf(" run %s ", runID)).SetBorder(true).SetBorderColor(colorBorder)

	v := &runWatchView{
		app:      a,
		runID:    runID,
		actionID: actionID,
		card:     card,
		logs:     newLogsView(a, runID),
	}
	v.ref.app = a.app

	v.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(card, 8, 0, false).
		AddItem(v.logs.Primitive(), 0, 1, true)
	return v
}

func (v *runWatchView) ID() string                 { return "watch/" + v.runID }
func (v *runWatchView) Title() string              { return "watch(" + v.runID + ")" }
func (v *runWatchView) Primitive() tview.Primitive { return v.flex }

func (v *runWatchView) Stop() {
	v.ref.stop()
	v.logs.Stop()
}

func (v *runWatchView) Ops() []Op {
	ops := []Op{
		{Rune: 'f', Name: "Follow on/off", Run: func(a *App, _ []Row) error {
			v.logs.follow = !v.logs.follow
			v.logs.render()
			return nil
		}},
		{Rune: 'd', Name: "Describe run", Run: func(a *App, _ []Row) error {
			if v.run == nil {
				return fmt.Errorf("the run has not been read yet")
			}
			a.push(newDescribeView(a, "run/"+v.runID, v.runID, v.run.Run))
			return nil
		}},
	}
	// Only offer the link once there is something to follow.
	if v.run != nil && len(v.run.GetLinkedEntities()) > 0 {
		ops = append(ops, Op{Rune: 'e', Name: "Linked entity", Run: func(a *App, _ []Row) error {
			link := v.run.GetLinkedEntities()[0]
			entity, err := a.client.GetEntity(a.ctx, link.Blueprint, link.Identifier)
			if err != nil {
				return err
			}
			a.push(newDescribeView(a, link.Blueprint+"/"+link.Identifier, link.Identifier, entity.Entity))
			return nil
		}})
	}
	return ops
}

// Start polls the run and tails its logs. The two are separate loops because
// the log tail is incremental and the status is a single small read.
func (v *runWatchView) Start(ctx context.Context) {
	v.logs.Start(ctx)
	v.ref.start(ctx, watchInterval,
		func(ctx context.Context) (any, error) { return v.app.client.GetActionRun(ctx, v.runID) },
		func(data any, err error) {
			if err != nil {
				v.lastErr = upsertAwareError(err).Error()
				v.render()
				return
			}
			v.lastErr = ""
			run, _ := data.(*client.ActionRun)
			v.run = run

			if run != nil && run.Run.Status != "IN_PROGRESS" {
				// Back off once terminal, then stop: a finished run has
				// nothing left to report and polling it forever is waste.
				v.settling++
				if v.settling > settleTicks {
					v.stopped = true
					v.ref.stop()
					v.logs.Stop()
					v.app.flash.show(flashInfo, "run %s finished: %s", v.runID, run.Run.Status)
				}
			}
			v.render()
		})
}

func (v *runWatchView) render() {
	var b strings.Builder
	field := func(name, value string) {
		fmt.Fprintf(&b, "[darkcyan::b]%-11s[white::-]%s\n", name+":", value)
	}

	field("Action", v.actionID)
	if v.run == nil {
		if v.lastErr != "" {
			fmt.Fprintf(&b, "[indianred]%s[-]\n", v.lastErr)
		} else {
			b.WriteString("[dimgray]reading the run…[-]\n")
		}
		v.card.SetText(b.String())
		return
	}

	run := v.run.Run
	status := run.Status
	if c := statusColor(status); c != tcell.ColorDefault {
		status = fmt.Sprintf("[%s]%s[-]", colorName(c), status)
	}
	field("Status", status)
	field("Blueprint", run.Blueprint.Identifier)
	field("Created", portfmt.DateTime(run.CreatedAt))
	if run.EndedAt != nil {
		field("Ended", portfmt.DateTime(*run.EndedAt))
	}
	if links := v.run.GetLinkedEntities(); len(links) > 0 {
		names := make([]string, 0, len(links))
		for _, l := range links {
			names = append(names, l.Blueprint+"/"+l.Identifier)
		}
		field("Entity", strings.Join(names, ", "))
	}
	if v.stopped {
		b.WriteString("[dimgray]⏹ watching stopped[-]\n")
	}
	if v.lastErr != "" {
		fmt.Fprintf(&b, "[indianred]%s[-]\n", v.lastErr)
	}
	v.card.SetText(b.String())
}

// colorName maps the few colours used in the status card back to the tag names
// tview's dynamic-colour markup understands.
func colorName(c tcell.Color) string {
	switch c {
	case colorInfo:
		return "palegreen"
	case colorWarn:
		return "orange"
	case colorError:
		return "indianred"
	default:
		return "white"
	}
}
