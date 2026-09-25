package upsert

import (
	"testing"

	"github.com/weka/portcli/internal/client"
)

func TestInputRef(t *testing.T) {
	for _, tc := range []struct {
		name, tmpl, want string
		ok               bool
	}{
		{"canonical form", "{{ .inputs.name }}", "name", true},
		{"no inner spaces", "{{.inputs.name}}", "name", true},
		{"surrounding whitespace", "  {{ .inputs.name }}  ", "name", true},
		{"underscores and digits", "{{ .inputs.my_input2 }}", "my_input2", true},
		{"a jq expression is not a plain input ref", "{{ .entity.properties.x }}", "", false},
		{"concatenation is not a plain ref", "prefix-{{ .inputs.name }}", "", false},
		{"a literal is not a ref", "static-id", "", false},
		{"empty", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := InputRef(tc.tmpl)
			if ok != tc.ok || got != tc.want {
				t.Errorf("got (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// action builds an ActionDetail whose upsert mapping uses the given identifier
// template.
func action(identifierTmpl string) *client.ActionDetail {
	a := &client.ActionDetail{}
	a.InvocationMethod.Type = "UPSERT_ENTITY"
	a.InvocationMethod.Mapping.Identifier = identifierTmpl
	return a
}

func TestTarget(t *testing.T) {
	props := map[string]any{"name": "svc-1", "blank": "", "num": 7.0}

	t.Run("override wins over the mapping", func(t *testing.T) {
		// Port applies an explicit identifier as the created entity's id, so it
		// has to beat whatever the mapping would have produced.
		if got := Target(action("{{ .inputs.name }}"), props, "chosen-id"); got != "chosen-id" {
			t.Errorf("got %q, want chosen-id", got)
		}
	})
	t.Run("reads the referenced input", func(t *testing.T) {
		if got := Target(action("{{ .inputs.name }}"), props, ""); got != "svc-1" {
			t.Errorf("got %q, want svc-1", got)
		}
	})
	t.Run("non-string input is rendered", func(t *testing.T) {
		if got := Target(action("{{ .inputs.num }}"), props, ""); got != "7" {
			t.Errorf("got %q, want 7", got)
		}
	})
	t.Run("static identifier is used as-is", func(t *testing.T) {
		if got := Target(action("fixed-name"), props, ""); got != "fixed-name" {
			t.Errorf("got %q, want fixed-name", got)
		}
	})
	t.Run("whitespace around a static identifier is trimmed", func(t *testing.T) {
		if got := Target(action("  fixed-name  "), props, ""); got != "fixed-name" {
			t.Errorf("got %q, want fixed-name", got)
		}
	})
	// An empty target is the signal that there is nothing to verify, which is
	// why an unevaluable template must not fall through to something wrong.
	t.Run("unevaluable template yields empty", func(t *testing.T) {
		for _, tmpl := range []string{
			"{{ .entity.identifier }}",
			"prefix-{{ .inputs.name }}",
			`{{ .inputs.a }}-{{ .inputs.b }}`,
		} {
			if got := Target(action(tmpl), props, ""); got != "" {
				t.Errorf("Target(%q) = %q, want empty", tmpl, got)
			}
		}
	})
	t.Run("input that resolved empty yields empty", func(t *testing.T) {
		if got := Target(action("{{ .inputs.blank }}"), props, ""); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("input the run never set yields empty", func(t *testing.T) {
		if got := Target(action("{{ .inputs.absent }}"), props, ""); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}
