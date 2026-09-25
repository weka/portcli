package tui

import "testing"

// dayTwoAction and createAction differ only in their operation, which is the
// one thing the target rule reads.
func actionWithOperation(t *testing.T, operation string) string {
	t.Helper()
	return `{
		"identifier":"restart","title":"Restart",
		"trigger":{"operation":"` + operation + `","blueprintIdentifier":"svc","userInputs":{
			"properties":{"reason":{"type":"string"}},
			"required":[],"order":["reason"]}},
		"invocationMethod":{"type":"WEBHOOK","blueprintIdentifier":"svc"}}`
}

// The form's "target entity" widget is only a mirror of v.entity, and tview's
// AddInputField installs the changed callback *after* it sets the text — so
// the initial value never reaches the callback. Anyone who later deletes the
// hand-assignment in buildFields and leans on the widget's value argument
// gets a form that looks right and submits a run with no target at all. This
// test is the only thing that catches that.
//
// newRunFormView stores the App but dereferences it only from callbacks, none
// of which fire during construction, so this needs no screen and no server.
func TestTargetEntityReachesTheValueThatIsSubmitted(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation string
		want      string
	}{
		{"day-2 action is aimed at the entity", "DAY-2", "dep-1"},
		{"delete action is aimed at the entity", "DELETE", "dep-1"},
		{"create action makes its own entity", "CREATE", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			action := actionFromJSON(t, actionWithOperation(t, tc.operation))
			v := newRunFormView(nil, action, runPrefill{TargetEntity: "dep-1"})

			// v.entity is what submit hands to ExecuteAction.
			if v.entity != tc.want {
				t.Errorf("v.entity = %q, want %q", v.entity, tc.want)
			}
		})
	}
}

// Two forms for the same action aimed at different entities must not collide:
// App.push reuses the body page whose ID matches, so an ID that ignored the
// target would show the first entity's half-filled form for the second.
func TestRunFormIDDistinguishesTargets(t *testing.T) {
	action := actionFromJSON(t, actionWithOperation(t, "DAY-2"))

	a := newRunFormView(nil, action, runPrefill{TargetEntity: "dep-a"}).ID()
	b := newRunFormView(nil, action, runPrefill{TargetEntity: "dep-b"}).ID()
	none := newRunFormView(nil, action, runPrefill{}).ID()

	if a == b {
		t.Errorf("both targets share the page ID %q", a)
	}
	if none != "run/restart" {
		t.Errorf("untargeted ID = %q, want the unchanged %q", none, "run/restart")
	}
}

// A retry rebuilds the form from the previous attempt, so the answers have to
// reach the widgets through the constructor. They used to be written into the
// values map afterwards, which left the widgets showing the schema defaults
// while the map held the carried-over answers.
func TestPreviousAnswersSurviveIntoTheRebuiltForm(t *testing.T) {
	action := actionFromJSON(t, actionWithOperation(t, "DAY-2"))

	v := newRunFormView(nil, action, runPrefill{
		Values: map[string]string{"reason": "disk pressure"},
	})

	if got := v.values["reason"]; got != "disk pressure" {
		t.Errorf("values[reason] = %q, want the carried-over answer", got)
	}
}
