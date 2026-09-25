package tui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/weka/portcli/internal/client"
)

// inputsView shows an action's input schema — the same table `action get`
// prints. A snapshot, since an action definition does not change while you
// are reading it.
type inputsView struct {
	app    *App
	action client.ActionDetail
	table  *tview.Table
}

func newInputsView(a *App, action client.ActionDetail) *inputsView {
	t := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	t.SetTitle(fmt.Sprintf(" inputs: %s ", action.Identifier)).
		SetBorder(true).SetBorderColor(colorBorder)
	t.SetSelectedStyle(tcell.StyleDefault.Background(colorSelected).Bold(true))

	for i, h := range []string{"INPUT", "TYPE", "REQUIRED", "DEFAULT", "ENUM"} {
		t.SetCell(0, i, tview.NewTableCell(h).
			SetTextColor(colorTitle).
			SetAttributes(tcell.AttrBold).
			SetSelectable(false).
			SetExpansion(1))
	}

	rows := inputRows(action)
	for r, cells := range rows {
		for c, text := range cells {
			cell := tview.NewTableCell(text).SetExpansion(1)
			if text == "<dynamic>" {
				cell.SetTextColor(colorWarn)
			}
			t.SetCell(r+1, c, cell)
		}
	}
	if len(rows) == 0 {
		t.SetCell(1, 0, tview.NewTableCell("[dimgray]this action takes no inputs").SetSelectable(false))
	}

	v := &inputsView{app: a, action: action, table: t}
	t.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			a.pop()
		}
	})
	return v
}

func (v *inputsView) ID() string                 { return "inputs/" + v.action.Identifier }
func (v *inputsView) Title() string              { return "inputs(" + v.action.Identifier + ")" }
func (v *inputsView) Primitive() tview.Primitive { return v.table }
func (v *inputsView) Start(context.Context)      {}
func (v *inputsView) Stop()                      {}

func (v *inputsView) Ops() []Op {
	return []Op{
		{
			Rune: 'd',
			Name: "Describe (raw)",
			Run: func(a *App, _ []Row) error {
				a.push(newDescribeView(a, "action/"+v.action.Identifier, v.action.Identifier, v.action))
				return nil
			},
		},
	}
}
