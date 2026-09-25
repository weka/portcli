package tui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/weka/portcli/internal/client"
)

// actionFromJSON builds an action from a payload shaped like the live
// /v1/actions response, so the fixtures below are the real thing rather than
// a guess at it.
func actionFromJSON(t *testing.T, body string) client.ActionDetail {
	t.Helper()
	var a client.ActionDetail
	if err := json.Unmarshal([]byte(body), &a); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	return a
}

func fieldByName(spec FormSpec, name string) (Field, bool) {
	for _, f := range spec.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

func TestBuildFormSpecFieldKinds(t *testing.T) {
	a := actionFromJSON(t, `{
		"identifier":"deploy","title":"Deploy",
		"trigger":{"operation":"CREATE","blueprintIdentifier":"svc","userInputs":{
			"properties":{
				"cloud":    {"type":"string","enum":["aws","gcp"],"default":"aws"},
				"template": {"type":"string","enum":{"jqQuery":".templates"}},
				"cluster":  {"type":"string","format":"entity","blueprint":"deployment"},
				"replicas": {"type":"number","minimum":1},
				"enabled":  {"type":"boolean","default":true},
				"tags":     {"type":"array"},
				"owner":    {"type":"string","default":{"jqQuery":".user.email"}},
				"name":     {"type":"string","pattern":"[a-z-]{3,}","title":"Service name"}
			},
			"required":["cloud","name"],
			"order":["cloud","template","cluster","replicas","enabled","tags","owner","name"]}},
		"invocationMethod":{"type":"UPSERT_ENTITY","blueprintIdentifier":"svc"}}`)

	spec := BuildFormSpec(&a)

	if !spec.IsUpsert() {
		t.Error("IsUpsert should be true for an UPSERT_ENTITY backend")
	}
	if spec.Blueprint != "svc" || spec.Operation != "CREATE" {
		t.Errorf("spec = %+v", spec)
	}

	for _, tc := range []struct {
		name string
		kind FieldKind
	}{
		{"cloud", FieldSelect},
		{"template", FieldText}, // a jq enum has no list to pick from
		{"cluster", FieldEntityRef},
		{"replicas", FieldNumber},
		{"enabled", FieldBool},
		{"tags", FieldJSON},
		{"name", FieldText},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := fieldByName(spec, tc.name)
			if !ok {
				t.Fatalf("%s missing from the spec", tc.name)
			}
			if f.Kind != tc.kind {
				t.Errorf("kind = %v, want %v", f.Kind, tc.kind)
			}
		})
	}

	t.Run("static default is prefilled", func(t *testing.T) {
		f, _ := fieldByName(spec, "cloud")
		if f.Default != "aws" {
			t.Errorf("default = %q, want aws", f.Default)
		}
	})

	// A default computed from a query over the calling user resolves to
	// nothing on client credentials, which is the usual cause of a dropped
	// upsert. It must not be prefilled with something invented, and the user
	// needs telling.
	t.Run("jq default is not prefilled but is explained", func(t *testing.T) {
		f, _ := fieldByName(spec, "owner")
		if f.Default != "" {
			t.Errorf("default = %q, want empty", f.Default)
		}
		if !strings.Contains(f.Hint, "server-side") {
			t.Errorf("hint does not explain the empty default: %q", f.Hint)
		}
	})

	t.Run("required fields are marked", func(t *testing.T) {
		f, _ := fieldByName(spec, "cloud")
		if !f.Required || !strings.HasSuffix(f.Label, "*") {
			t.Errorf("required field not marked: %+v", f)
		}
	})

	t.Run("entity refs carry their blueprint", func(t *testing.T) {
		f, _ := fieldByName(spec, "cluster")
		if f.EntityBP != "deployment" {
			t.Errorf("EntityBP = %q", f.EntityBP)
		}
	})

	t.Run("title is shown alongside the name", func(t *testing.T) {
		f, _ := fieldByName(spec, "name")
		if !strings.Contains(f.Label, "Service name") {
			t.Errorf("label = %q", f.Label)
		}
	})

	t.Run("constraints are carried through", func(t *testing.T) {
		if f, _ := fieldByName(spec, "replicas"); f.Minimum == nil || *f.Minimum != 1 {
			t.Errorf("minimum not carried: %+v", f)
		}
		if f, _ := fieldByName(spec, "name"); f.Pattern != "[a-z-]{3,}" {
			t.Errorf("pattern not carried: %+v", f)
		}
	})
}

// Conditional inputs are shown, not hidden. Hiding them would reproduce the
// exact failure this tool exists to diagnose: a hidden input whose default
// resolves empty silently drops the upsert.
func TestConditionalInputsAreShownAndGroupedLast(t *testing.T) {
	a := actionFromJSON(t, `{
		"identifier":"deploy",
		"trigger":{"userInputs":{
			"properties":{
				"always":  {"type":"string"},
				"maybe":   {"type":"string","visible":{"jqQuery":".form.x == true"}},
				"plain":   {"type":"string","visible":true},
				"other":   {"type":"string"}
			},
			"order":["maybe","always","plain","other"]}},
		"invocationMethod":{"type":"WEBHOOK"}}`)

	spec := BuildFormSpec(&a)

	maybe, _ := fieldByName(spec, "maybe")
	if !maybe.Conditional {
		t.Error("a jq visibility must mark the field conditional")
	}
	if !strings.Contains(maybe.Hint, "conditionally") {
		t.Errorf("hint = %q", maybe.Hint)
	}
	if plain, _ := fieldByName(spec, "plain"); plain.Conditional {
		t.Error("a static visible:true is not conditional")
	}

	// Conditional fields sort to the end, and the order among the rest is the
	// action's declared order.
	var names []string
	for _, f := range spec.Fields {
		names = append(names, f.Name)
	}
	if got := names[len(names)-1]; got != "maybe" {
		t.Errorf("field order = %v, want the conditional one last", names)
	}
	if !reflect.DeepEqual(names[:3], []string{"always", "plain", "other"}) {
		t.Errorf("declared order not preserved among unconditional fields: %v", names)
	}
}

func TestBuildPayload(t *testing.T) {
	spec := FormSpec{Fields: []Field{
		{Name: "cloud", Kind: FieldSelect, Required: true},
		{Name: "replicas", Kind: FieldNumber},
		{Name: "enabled", Kind: FieldBool},
		{Name: "tags", Kind: FieldJSON},
		{Name: "note", Kind: FieldText},
	}}

	t.Run("types are converted", func(t *testing.T) {
		got, errs := BuildPayload(spec, map[string]string{
			"cloud":    "aws",
			"replicas": "3",
			"enabled":  "true",
			"tags":     `["a","b"]`,
			"note":     "hello",
		})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		want := map[string]any{
			"cloud":    "aws",
			"replicas": 3.0,
			"enabled":  true,
			"tags":     []any{"a", "b"},
			"note":     "hello",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})

	// Sending "" would be a real value to Port and would override the default
	// it would otherwise apply, so an untouched optional field is omitted.
	t.Run("empty optional fields are omitted, not sent blank", func(t *testing.T) {
		got, errs := BuildPayload(spec, map[string]string{"cloud": "aws", "note": ""})
		if len(errs) > 0 {
			t.Fatalf("unexpected errors: %v", errs)
		}
		if _, present := got["note"]; present {
			t.Errorf("empty optional field was sent: %#v", got)
		}
	})

	t.Run("every problem is reported, not just the first", func(t *testing.T) {
		_, errs := BuildPayload(spec, map[string]string{
			"replicas": "not-a-number",
			"tags":     "{oops",
		})
		if len(errs) < 3 { // missing cloud, bad number, bad JSON
			t.Errorf("got %d errors, want all of them: %v", len(errs), errs)
		}
	})

	t.Run("required and empty is an error", func(t *testing.T) {
		_, errs := BuildPayload(spec, map[string]string{"cloud": "  "})
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "cloud is required") {
			t.Errorf("errs = %v", errs)
		}
	})

	t.Run("bounds are enforced", func(t *testing.T) {
		min, max := 2.0, 5.0
		bounded := FormSpec{Fields: []Field{{Name: "n", Kind: FieldNumber, Minimum: &min, Maximum: &max}}}
		if _, errs := BuildPayload(bounded, map[string]string{"n": "1"}); len(errs) != 1 {
			t.Errorf("below minimum accepted: %v", errs)
		}
		if _, errs := BuildPayload(bounded, map[string]string{"n": "9"}); len(errs) != 1 {
			t.Errorf("above maximum accepted: %v", errs)
		}
		if _, errs := BuildPayload(bounded, map[string]string{"n": "3"}); len(errs) != 0 {
			t.Errorf("in-range rejected: %v", errs)
		}
	})

	t.Run("pattern is enforced", func(t *testing.T) {
		p := FormSpec{Fields: []Field{{Name: "name", Kind: FieldText, Pattern: "^[a-z]+$"}}}
		if _, errs := BuildPayload(p, map[string]string{"name": "ABC"}); len(errs) != 1 {
			t.Errorf("pattern not enforced: %v", errs)
		}
		if _, errs := BuildPayload(p, map[string]string{"name": "abc"}); len(errs) != 0 {
			t.Errorf("valid value rejected: %v", errs)
		}
	})

	// Port's patterns are ECMA; Go's regexp is RE2, which rejects lookahead
	// among other things. A pattern we cannot compile is Port's business, not
	// the user's, so the value goes through and the API decides.
	t.Run("an uncompilable pattern does not block submission", func(t *testing.T) {
		p := FormSpec{Fields: []Field{{Name: "name", Kind: FieldText, Pattern: `\d+(?=%)`}}}
		got, errs := BuildPayload(p, map[string]string{"name": "anything"})
		if len(errs) != 0 {
			t.Fatalf("blocked by a pattern we cannot evaluate: %v", errs)
		}
		if got["name"] != "anything" {
			t.Errorf("value dropped: %#v", got)
		}
	})
}

// A CREATE action makes its own entity, so aiming a run at an existing one is
// meaningless; everything else acts on an entity that is already there. The
// empty case is a v1-shaped trigger, and treating those as day-2 is the safer
// default — Port tolerates a stray target far better than a dropped one.
func TestAcceptsTargetEntity(t *testing.T) {
	for _, tc := range []struct {
		operation string
		want      bool
	}{
		{"CREATE", false},
		{"create", false}, // Port's casing is not something to depend on
		{"DAY-2", true},
		{"DELETE", true},
		{"", true},
	} {
		t.Run("op="+tc.operation, func(t *testing.T) {
			spec := FormSpec{Operation: tc.operation}
			if got := spec.AcceptsTargetEntity(); got != tc.want {
				t.Errorf("AcceptsTargetEntity() = %v, want %v", got, tc.want)
			}
		})
	}
}
