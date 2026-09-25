package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/upsert"
)

// runFormView collects an action's inputs and launches it.
//
// A full-screen page, not a tview.Modal: a Modal sizes itself to its text and
// hosts only buttons, and actions here run to dozens of inputs.
type runFormView struct {
	app  *App
	spec FormSpec
	form *tview.Form
	flex *tview.Flex

	values map[string]string
	// entityCache holds a blueprint's identifiers for an entity picker, keyed
	// by blueprint and filled on first use rather than up front.
	entityCache map[string][]string
	// advanced mirror the CLI's --run-as / --entity / --id flags.
	runAs, entity, identifier string
	pre                       runPrefill
}

// runPrefill is what a caller already knows about the run before the form is
// built: the entity the user had selected, and the answers from an attempt
// being retried. Both used to be written into the view *after* construction,
// which left the widgets and the values map disagreeing until the user
// retyped. Going through the constructor is what keeps the two in step.
type runPrefill struct {
	TargetEntity string            // --entity; withheld from CREATE actions
	Values       map[string]string // carried over from a previous attempt
}

func newRunFormView(a *App, action client.ActionDetail, pre runPrefill) *runFormView {
	v := &runFormView{
		app:         a,
		spec:        BuildFormSpec(&action),
		values:      map[string]string{},
		entityCache: map[string][]string{},
		pre:         pre,
	}

	v.form = tview.NewForm()
	v.form.SetItemPadding(0)
	v.form.SetFieldBackgroundColor(tcell.ColorBlack)

	v.buildFields()
	v.form.
		AddButton("Run", v.submit).
		AddButton("Cancel", func() { a.pop() })
	v.form.SetCancelFunc(func() { a.pop() })

	title := fmt.Sprintf(" run %s ", v.spec.ActionID)
	v.form.SetTitle(title).SetBorder(true).SetBorderColor(colorBorder)

	// Sized from the text rather than a constant: the summary grows by a line
	// when the run has a target, and a fixed height would cut it off.
	lines := v.summaryLines()
	header := tview.NewTextView().SetDynamicColors(true)
	header.SetText(strings.Join(lines, "\n"))

	v.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, len(lines)+1, 0, false).
		AddItem(v.form, 0, 1, true)
	return v
}

// summaryLines states what will happen, including the warning that matters
// most: an UPSERT_ENTITY action leaves no run to inspect. Called after
// buildFields, which is what decides whether the target was accepted.
func (v *runFormView) summaryLines() []string {
	lines := []string{
		fmt.Sprintf("[darkcyan]action[-]    %s   [darkcyan]blueprint[-] %s   [darkcyan]operation[-] %s",
			v.spec.ActionID, v.spec.Blueprint, v.spec.Operation),
		fmt.Sprintf("[darkcyan]backend[-]   %s", v.spec.Backend),
	}
	switch {
	case v.entity != "":
		lines = append(lines, fmt.Sprintf("[darkcyan]target[-]    %s", v.entity))
	case v.pre.TargetEntity != "":
		// Offered a target and declined it. Saying so beats leaving the user
		// to wonder why the entity they selected is nowhere on the screen.
		lines = append(lines, fmt.Sprintf(
			"[orange]%s creates its own entity, so %s is not used as a target.[-]",
			v.spec.ActionID, v.pre.TargetEntity))
	}
	if v.spec.IsUpsert() {
		lines = append(lines,
			"[orange]Port writes the entity itself and keeps no run record — the entity is the only evidence.[-]")
	}
	return lines
}

func (v *runFormView) buildFields() {
	conditionalAdded := false
	for _, f := range v.spec.Fields {
		if f.Conditional && !conditionalAdded {
			conditionalAdded = true
			// A separator rather than hiding them: see BuildFormSpec.
			v.form.AddTextView("── shown conditionally by Port ──",
				"these are submitted too; a hidden one that resolves empty is how an upsert gets dropped",
				0, 2, true, false)
		}
		v.addField(f)
	}

	v.form.AddTextView("── advanced ──", "equivalents of --run-as, --entity and --id", 0, 2, true, false)
	v.form.AddInputField("run as (email)", "", fieldWidth, nil, func(s string) { v.runAs = s })
	if v.spec.AcceptsTargetEntity() {
		// AddInputField installs the changed callback *after* it sets the
		// text, so the initial value never reaches it. The field below only
		// mirrors v.entity; the value it mirrors has to be set by hand, or the
		// run goes out with no target at all.
		v.entity = v.pre.TargetEntity
	}
	v.form.AddInputField("target entity", v.entity, fieldWidth, nil, func(s string) { v.entity = s })
	v.form.AddInputField("identifier override", "", fieldWidth, nil, func(s string) { v.identifier = s })
}

// initial is the value a field starts with: whatever a previous attempt left
// for it, else the action's own default. Without this a retry would rebuild
// every widget from the schema and throw away the answers it was carrying.
func (v *runFormView) initial(name, def string) string {
	if got := v.pre.Values[name]; got != "" {
		return got
	}
	return def
}

func (v *runFormView) addField(f Field) {
	label := f.Label
	name := f.Name
	start := v.initial(name, f.Default)

	switch f.Kind {
	case FieldSelect:
		options := f.Options
		selected := 0
		if !f.Required {
			// A blank first option is how an optional enum is left unset, so
			// Port applies its own default instead of a value we invented.
			options = append([]string{""}, options...)
		}
		for i, o := range options {
			if o == start && start != "" {
				selected = i
			}
		}
		v.values[name] = options[selected]
		v.form.AddDropDown(label, options, selected, func(option string, _ int) {
			v.values[name] = option
		})

	case FieldBool:
		checked := start == "true"
		v.values[name] = strconv.FormatBool(checked)
		v.form.AddCheckbox(label, checked, func(b bool) {
			v.values[name] = strconv.FormatBool(b)
		})

	case FieldNumber:
		v.values[name] = start
		v.form.AddInputField(label, start, 16, tview.InputFieldFloat, func(s string) {
			v.values[name] = s
		})

	case FieldEntityRef:
		v.values[name] = start
		input := tview.NewInputField().SetLabel(label).SetText(start).SetFieldWidth(fieldWidth)
		input.SetChangedFunc(func(s string) { v.values[name] = s })
		// Fetched on first keystroke, not up front: a form with several entity
		// inputs would otherwise fire a request per input before the user has
		// typed anything.
		input.SetAutocompleteFunc(func(current string) []string {
			return withPrefix(v.entityOptions(f.EntityBP), current)
		})
		v.form.AddFormItem(input)

	case FieldJSON:
		v.values[name] = start
		v.form.AddTextArea(label, start, 0, 3, 0, func(s string) { v.values[name] = s })

	default:
		v.values[name] = start
		v.form.AddInputField(label, start, fieldWidth, nil, func(s string) { v.values[name] = s })
	}

	if f.Hint != "" {
		v.form.AddTextView("", "  "+truncate(f.Hint, hintWidth), 0, 1, true, false)
	}
}

// fieldWidth and hintWidth keep a row inside the form's border on a normal
// terminal; a row that overflows makes the whole box look broken.
const (
	fieldWidth = 30
	hintWidth  = 70
)

// truncate shortens s to at most n runes, marking where it was cut.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

// entityOptions returns a blueprint's identifiers, fetching once per
// blueprint. Called from the UI goroutine by the autocomplete callback, so
// the request is synchronous and deliberately bounded by the client's timeout.
func (v *runFormView) entityOptions(blueprint string) []string {
	if cached, ok := v.entityCache[blueprint]; ok {
		return cached
	}
	entities, err := v.app.client.SearchEntities(v.app.ctx, blueprint)
	if err != nil {
		// Cache the failure as empty so a broken blueprint does not retry on
		// every keystroke.
		v.entityCache[blueprint] = nil
		v.app.flash.show(flashWarn, "cannot list %s entities: %v", blueprint, err)
		return nil
	}
	ids := make([]string, 0, len(entities))
	for _, e := range entities {
		ids = append(ids, e.Identifier)
	}
	v.entityCache[blueprint] = ids
	return ids
}

func (v *runFormView) submit() {
	props, errs := BuildPayload(v.spec, v.values)
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = "• " + e.Error()
		}
		v.app.showError(fmt.Errorf("fix these first:\n%s", strings.Join(msgs, "\n")))
		return
	}

	result, err := v.app.client.ExecuteAction(v.app.ctx, v.spec.ActionID, props, v.runAs, v.entity, v.identifier)
	if err != nil {
		v.app.showError(fmt.Errorf("could not start %s: %w", v.spec.ActionID, err))
		return
	}

	v.app.flash.show(flashInfo, "started %s (run %s)", v.spec.ActionID, result.Run.ID)

	if v.spec.IsUpsert() {
		// No run watcher: the id 404s from the moment it is returned, so
		// polling status or logs would show a permanent error either way.
		// The prefill carries this attempt's answers and target into the
		// verification, which is where a retry rebuilds the form from them.
		v.app.push(newUpsertVerifyView(v.app, v.spec, upsert.Request{
			ActionID:   v.spec.ActionID,
			RunID:      result.Run.ID,
			Identifier: v.identifier,
			RunProps:   result.Run.Properties,
		}, runPrefill{TargetEntity: v.entity, Values: v.values}))
		return
	}
	v.app.push(newRunWatchView(v.app, result.Run.ID, v.spec.ActionID))
}

// ID includes the target. push reuses the body page whose ID matches, so two
// forms for the same action aimed at different entities would otherwise share
// one half-filled form.
func (v *runFormView) ID() string {
	if v.entity != "" {
		return "run/" + v.spec.ActionID + "@" + v.entity
	}
	return "run/" + v.spec.ActionID
}

func (v *runFormView) Title() string              { return "run(" + v.spec.ActionID + ")" }
func (v *runFormView) Primitive() tview.Primitive { return v.flex }
func (v *runFormView) Start(context.Context)      {}
func (v *runFormView) Stop()                      {}

// Ops is empty: while a form has focus the global capture hands every key to
// the focused field, so a row operation here would be unreachable anyway.
func (v *runFormView) Ops() []Op { return nil }
