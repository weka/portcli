package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// actionsResource lists the self-service catalog.
//
// One ListActions call answers everything: Port returns complete action
// objects there, so a row already carries the trigger inputs and the
// invocation mapping. Nothing here needs a follow-up GetAction.
type actionsResource struct {
	blueprint string // optional, narrows to actions on one blueprint
}

func init() {
	Register("actions", []string{"act", "a"}, func(args []string) (Resource, error) {
		r := &actionsResource{}
		if len(args) > 0 {
			r.blueprint = args[0]
		}
		return r, nil
	})
}

func (r *actionsResource) Kind() string { return "actions" }

func (r *actionsResource) ID() string {
	if r.blueprint != "" {
		return "actions " + r.blueprint
	}
	return "actions"
}

func (r *actionsResource) Title() string {
	if r.blueprint != "" {
		return fmt.Sprintf("actions(%s)", r.blueprint)
	}
	return "actions"
}

func (r *actionsResource) Columns() []Column {
	return []Column{
		{Name: "IDENTIFIER"},
		{Name: "TITLE"},
		{Name: "BLUEPRINT"},
		{Name: "OPERATION"},
		{Name: "BACKEND"},
		{Name: "INPUTS"},
		{Name: "APPROVAL", Wide: true},
	}
}

func (r *actionsResource) List(ctx context.Context, c *client.Client) ([]Row, error) {
	actions, err := c.ListActions(ctx)
	if err != nil {
		return nil, err
	}

	rows := make([]Row, 0, len(actions))
	for _, a := range actions {
		if r.blueprint != "" && !strings.EqualFold(a.Trigger.BlueprintIdentifier, r.blueprint) {
			continue
		}
		approval := ""
		if a.RequiredApproval {
			approval = "required"
		}
		rows = append(rows, Row{
			ID:  a.Identifier,
			Obj: a,
			Cells: []string{
				a.Identifier,
				a.Title,
				a.Trigger.BlueprintIdentifier,
				a.Trigger.Operation,
				a.InvocationMethod.Type,
				inputSummary(a),
				approval,
			},
		})
	}
	return rows, nil
}

// inputSummary counts an action's inputs, calling out the required ones and
// the ones Port computes server-side — a dynamic enum or jq default is the
// usual reason a run needs a value passed explicitly.
func inputSummary(a client.ActionDetail) string {
	props := a.Trigger.UserInputs.Properties
	if len(props) == 0 {
		return "none"
	}
	dynamic := 0
	for _, p := range props {
		if _, isDynamic := p.EnumValues(); isDynamic {
			dynamic++
		}
	}
	out := strconv.Itoa(len(props))
	if n := len(a.Trigger.UserInputs.Required); n > 0 {
		out += fmt.Sprintf(" (%d required", n)
		if dynamic > 0 {
			out += fmt.Sprintf(", %d dynamic", dynamic)
		}
		out += ")"
	} else if dynamic > 0 {
		out += fmt.Sprintf(" (%d dynamic)", dynamic)
	}
	return out
}

func (r *actionsResource) Ops() []Op {
	return []Op{
		{
			Key:  tcell.KeyEnter,
			Name: "Inputs",
			Run:  func(a *App, rows []Row) error { return showInputs(a, rows) },
		},
		{
			Rune: 'r',
			Name: "Run",
			Run: func(a *App, rows []Row) error {
				action, ok := rows[0].Obj.(client.ActionDetail)
				if !ok {
					return fmt.Errorf("unexpected row contents for %s", rows[0].ID)
				}
				// The list payload already carries the trigger inputs, so the
				// form opens without another request.
				a.push(newRunFormView(a, action))
				return nil
			},
		},
		{
			Rune: 'd',
			Name: "Describe",
			Run: func(a *App, rows []Row) error {
				a.push(newDescribeView(a, "action/"+rows[0].ID, rows[0].ID, rows[0].Obj))
				return nil
			},
		},
	}
}

// showInputs renders an action's input schema the way `action get` does, which
// is the question someone selecting an action is actually asking.
func showInputs(a *App, rows []Row) error {
	action, ok := rows[0].Obj.(client.ActionDetail)
	if !ok {
		return fmt.Errorf("unexpected row contents for %s", rows[0].ID)
	}
	a.push(newInputsView(a, action))
	return nil
}

// inputRows renders an action's inputs as table lines. Pure, so the mapping
// from schema to what a user reads is testable.
func inputRows(action client.ActionDetail) [][]string {
	inputs := action.Trigger.UserInputs
	required := make(map[string]bool, len(inputs.Required))
	for _, name := range inputs.Required {
		required[name] = true
	}

	var out [][]string
	for _, name := range portfmt.InputOrder(inputs.Order, inputs.Properties) {
		p := inputs.Properties[name]
		req := ""
		if required[name] {
			req = "yes"
		}
		values, dynamic := p.EnumValues()
		enum := portfmt.EnumPreview(values)
		if dynamic {
			// Port computes these at run time from a query we cannot evaluate,
			// so an empty cell here would read as "accepts anything".
			enum = "<dynamic>"
		}
		out = append(out, []string{name, p.Type, req, portfmt.AnyToString(p.Default), enum})
	}
	return out
}
