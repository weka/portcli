package tui

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// describeView shows one object as indented JSON. It is a View but not a
// Resource: there are no rows, columns, sorting or marks to speak of, and
// forcing it into the table abstraction would mean inventing all four.
type describeView struct {
	app   *App
	id    string
	title string
	view  *tview.TextView
}

func newDescribeView(a *App, id, title string, obj any) *describeView {
	v := tview.NewTextView().SetScrollable(true).SetWrap(false)
	v.SetTitle(fmt.Sprintf(" %s ", title)).SetBorder(true).SetBorderColor(colorBorder)

	out, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		v.SetText(fmt.Sprintf("cannot render this object: %v", err))
	} else {
		v.SetText(string(out))
	}

	d := &describeView{app: a, id: "describe/" + id, title: title, view: v}
	v.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			a.pop()
		}
	})
	return d
}

func (d *describeView) ID() string                 { return d.id }
func (d *describeView) Title() string              { return "describe(" + d.title + ")" }
func (d *describeView) Primitive() tview.Primitive { return d.view }
func (d *describeView) Start(context.Context)      {} // a snapshot; nothing to poll
func (d *describeView) Stop()                      {}

func (d *describeView) Ops() []Op {
	return []Op{{Rune: 'y', Name: "Copy to clipboard", Run: func(a *App, _ []Row) error {
		return copyToClipboard(a, d.view.GetText(true))
	}}}
}

// copyToClipboard writes via OSC 52, which the terminal emulator forwards to
// the system clipboard. It costs no dependency and works over ssh, where a
// clipboard library on this end would be writing to the wrong machine.
func copyToClipboard(a *App, text string) error {
	if err := osc52(text); err != nil {
		return err
	}
	a.flash.show(flashInfo, "copied %d bytes", len(text))
	return nil
}
