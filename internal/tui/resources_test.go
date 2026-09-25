package tui

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/weka/portcli/internal/client"
)

func blueprintWith(props map[string]string) *client.BlueprintDetail {
	bp := &client.BlueprintDetail{}
	bp.Schema.Properties = map[string]client.BlueprintProperty{}
	for name, typ := range props {
		bp.Schema.Properties[name] = client.BlueprintProperty{Type: typ}
	}
	return bp
}

func TestScalarColumns(t *testing.T) {
	t.Run("well-known names come first", func(t *testing.T) {
		// Someone scanning a list of entities is looking for status before
		// they are looking for anything alphabetical.
		got := scalarColumns(blueprintWith(map[string]string{
			"alpha": "string", "status": "string", "owner": "string", "zulu": "string",
		}))
		want := []string{"status", "owner", "alpha", "zulu"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("non-scalars are excluded", func(t *testing.T) {
		// An object or array renders as a wall of JSON and crowds out the rest
		// of the row.
		got := scalarColumns(blueprintWith(map[string]string{
			"status": "string", "config": "object", "tags": "array", "count": "number", "ok": "boolean",
		}))
		for _, unwanted := range []string{"config", "tags"} {
			if contains(got, unwanted) {
				t.Errorf("%q should not be a column: %v", unwanted, got)
			}
		}
		for _, wanted := range []string{"status", "count", "ok"} {
			if !contains(got, wanted) {
				t.Errorf("%q should be a column: %v", wanted, got)
			}
		}
	})

	t.Run("capped so the table stays readable", func(t *testing.T) {
		props := map[string]string{}
		for _, n := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
			props[n] = "string"
		}
		if got := scalarColumns(blueprintWith(props)); len(got) != maxAutoColumns {
			t.Errorf("got %d columns, want %d: %v", len(got), maxAutoColumns, got)
		}
	})

	t.Run("case-insensitive match on preferred names", func(t *testing.T) {
		got := scalarColumns(blueprintWith(map[string]string{"Status": "string", "aaa": "string"}))
		if len(got) == 0 || got[0] != "Status" {
			t.Errorf("got %v, want Status first", got)
		}
	})

	t.Run("no scalar properties yields nothing", func(t *testing.T) {
		if got := scalarColumns(blueprintWith(map[string]string{"cfg": "object"})); len(got) != 0 {
			t.Errorf("got %v, want none", got)
		}
	})
}

func TestPropertiesSeenFallback(t *testing.T) {
	// Used when the blueprint schema cannot be read: take whatever keys the
	// entities actually carry rather than showing identifier alone.
	entities := []client.EntitySummary{
		{Properties: map[string]any{"zulu": 1, "status": "OK"}},
		{Properties: map[string]any{"alpha": true}},
	}
	got := propertiesSeen(entities)
	if len(got) == 0 || got[0] != "status" {
		t.Errorf("got %v, want status first", got)
	}
	if len(got) > maxAutoColumns {
		t.Errorf("got %d columns, want at most %d", len(got), maxAutoColumns)
	}
}

// actionWith builds an ActionDetail with the given inputs.
func actionWith(t *testing.T, requiredNames []string, inputs map[string]string) client.ActionDetail {
	t.Helper()
	a := client.ActionDetail{Identifier: "act"}
	a.Trigger.UserInputs.Properties = map[string]client.ActionInput{}
	a.Trigger.UserInputs.Required = requiredNames
	for name, raw := range inputs {
		var in client.ActionInput
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			t.Fatalf("bad fixture for %s: %v", name, err)
		}
		a.Trigger.UserInputs.Properties[name] = in
		a.Trigger.UserInputs.Order = append(a.Trigger.UserInputs.Order, name)
	}
	return a
}

func TestInputSummary(t *testing.T) {
	t.Run("no inputs", func(t *testing.T) {
		if got := inputSummary(actionWith(t, nil, nil)); got != "none" {
			t.Errorf("got %q, want none", got)
		}
	})

	t.Run("counts required", func(t *testing.T) {
		a := actionWith(t, []string{"cloud"}, map[string]string{
			"cloud": `{"type":"string","enum":["aws"]}`,
			"name":  `{"type":"string"}`,
		})
		if got := inputSummary(a); got != "2 (1 required)" {
			t.Errorf("got %q", got)
		}
	})

	// A dynamic enum is the thing worth flagging before running an action: its
	// values come from a server-side query, so it cannot be picked from a list.
	t.Run("counts dynamic enums", func(t *testing.T) {
		a := actionWith(t, []string{"tmpl"}, map[string]string{
			"tmpl": `{"type":"string","enum":{"jqQuery":".x"}}`,
		})
		if got := inputSummary(a); !strings.Contains(got, "1 dynamic") {
			t.Errorf("got %q, want it to mention a dynamic input", got)
		}
	})
}

func TestInputRows(t *testing.T) {
	a := actionWith(t, []string{"cloud"}, map[string]string{
		"cloud": `{"type":"string","enum":["aws","gcp"],"default":"aws"}`,
		"tmpl":  `{"type":"string","enum":{"jqQuery":".templates"}}`,
		"count": `{"type":"number"}`,
	})
	rows := inputRows(a)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}

	// Declared order is preserved, which is the order the action author chose.
	byName := map[string][]string{}
	for _, r := range rows {
		byName[r[0]] = r
	}

	if got := byName["cloud"]; got[1] != "string" || got[2] != "yes" || got[3] != "aws" || got[4] != "aws,gcp" {
		t.Errorf("cloud row = %v", got)
	}
	if got := byName["tmpl"]; got[4] != "<dynamic>" {
		t.Errorf("a jq enum must be named, not left blank: %v", got)
	}
	if got := byName["count"]; got[2] != "" || got[4] != "" {
		t.Errorf("count row = %v", got)
	}
}

func TestResourceIDsAreStableAndReparsable(t *testing.T) {
	// A view's ID is persisted as the last view and fed back through
	// ParseCommand on the next start, so it has to survive the round trip.
	for _, tc := range []struct {
		kind string
		args []string
	}{
		{"blueprints", nil},
		{"entities", []string{"deployment"}},
		{"actions", nil},
		{"actions", []string{"deployment"}},
		{"runs", nil},
		{"runs", []string{"deployment", "dep-1"}},
	} {
		name := tc.kind + "/" + strings.Join(tc.args, ",")
		t.Run(name, func(t *testing.T) {
			res, err := Resolve(tc.kind, tc.args)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			cmd, ok := ParseCommand(res.ID())
			if !ok {
				t.Fatalf("ID %q does not parse", res.ID())
			}
			again, err := Resolve(cmd.Kind, cmd.Args)
			if err != nil {
				t.Fatalf("reparsed %q: %v", res.ID(), err)
			}
			if again.ID() != res.ID() {
				t.Errorf("round trip changed the ID: %q -> %q", res.ID(), again.ID())
			}
		})
	}
}

func TestEntitiesRequiresABlueprint(t *testing.T) {
	if _, err := Resolve("entities", nil); err == nil {
		t.Error("entities without a blueprint should explain itself, not succeed")
	}
}

func TestEntitiesServerFilterIsPartOfTheID(t *testing.T) {
	res, err := Resolve("entities", []string{"deployment"})
	if err != nil {
		t.Fatal(err)
	}
	f := res.(*entitiesResource)
	if err := f.SetServerFilter("status=Failed"); err != nil {
		t.Fatal(err)
	}
	// Two filtered views of one blueprint are different views, so the filter
	// has to be in the key that separates them on the stack.
	if got := res.ID(); got != "entities deployment status=Failed" {
		t.Errorf("ID = %q", got)
	}
	if err := f.SetServerFilter("nonsense"); err == nil {
		t.Error("a filter without field=value should be rejected")
	}
}
