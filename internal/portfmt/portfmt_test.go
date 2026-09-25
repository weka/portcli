package portfmt

import (
	"reflect"
	"testing"

	"github.com/weka/portcli/internal/client"
)

func TestAnyToString(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"nil is empty, not null", nil, ""},
		{"string passes through unquoted", "ready", "ready"},
		{"empty string", "", ""},
		{"number", 42.0, "42"},
		{"bool", true, "true"},
		{"object", map[string]any{"a": 1.0}, `{"a":1}`},
		{"array", []any{"x", "y"}, `["x","y"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AnyToString(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPropStr(t *testing.T) {
	props := map[string]any{"status": "OK", "count": 3.0, "nulled": nil}
	for _, tc := range []struct {
		name, key, want string
	}{
		{"present string", "status", "OK"},
		{"present number", "count", "3"},
		{"explicit null", "nulled", ""},
		{"missing key", "absent", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := PropStr(props, tc.key); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("nil map", func(t *testing.T) {
		if got := PropStr(nil, "status"); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

func TestCamelToSnake(t *testing.T) {
	for in, want := range map[string]string{
		"createdAt":   "created_At",
		"status":      "status",
		"":            "",
		"deployedFor": "deployed_For",
	} {
		if got := CamelToSnake(in); got != want {
			t.Errorf("CamelToSnake(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDateAndDateTime(t *testing.T) {
	t.Run("Date", func(t *testing.T) {
		for in, want := range map[string]string{
			"2026-09-25T04:44:40.647Z": "2026-09-25",
			"2026-09-25":               "2026-09-25",
			"":                         "",
		} {
			if got := Date(in); got != want {
				t.Errorf("Date(%q) = %q, want %q", in, got, want)
			}
		}
	})
	t.Run("DateTime", func(t *testing.T) {
		for in, want := range map[string]string{
			"2026-09-25T04:44:40.647Z": "2026-09-25 04:44:40",
			"2026-09-25T04:44:40":      "2026-09-25 04:44:40",
			"2026-09-25":               "2026-09-25",
			"":                         "",
		} {
			if got := DateTime(in); got != want {
				t.Errorf("DateTime(%q) = %q, want %q", in, got, want)
			}
		}
	})
}

func TestEntityCol(t *testing.T) {
	e := client.EntitySummary{
		Identifier: "e1",
		CreatedAt:  "2026-09-25T04:44:40.647Z",
		CreatedBy:  "someone@example.com",
		Properties: map[string]any{"status": "OK"},
	}
	for _, tc := range []struct {
		name, col, want string
	}{
		// The two top-level fields are resolved before the property bag, and
		// the lookup is case-insensitive because column names come from a flag.
		{"createdAt is trimmed to a date", "createdAt", "2026-09-25"},
		{"createdAt case-insensitive", "CREATEDAT", "2026-09-25"},
		{"createdBy", "createdBy", "someone@example.com"},
		{"a real property", "status", "OK"},
		{"an unknown column is empty", "nope", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := EntityCol(e, tc.col); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnumPreview(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := EnumPreview(nil); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
	t.Run("short list is joined", func(t *testing.T) {
		if got := EnumPreview([]any{"aws", "gcp"}); got != "aws,gcp" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("long list is truncated with a count", func(t *testing.T) {
		vals := []any{"a", "b", "c", "d", "e", "f", "g", "h"}
		want := "a,b,c,d,e,f,… (+2 more)"
		if got := EnumPreview(vals); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestInputOrder(t *testing.T) {
	props := map[string]client.ActionInput{
		"zebra": {}, "cloud": {}, "name": {}, "alpha": {},
	}
	t.Run("declared order first, remainder sorted", func(t *testing.T) {
		got := InputOrder([]string{"name", "cloud"}, props)
		want := []string{"name", "cloud", "alpha", "zebra"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("names the action does not have are dropped", func(t *testing.T) {
		got := InputOrder([]string{"name", "ghost"}, props)
		want := []string{"name", "alpha", "cloud", "zebra"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("no declared order sorts everything", func(t *testing.T) {
		got := InputOrder(nil, props)
		want := []string{"alpha", "cloud", "name", "zebra"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("a repeated name is not emitted twice", func(t *testing.T) {
		got := InputOrder([]string{"name", "name"}, props)
		if len(got) != len(props) {
			t.Errorf("got %v (%d entries), want %d", got, len(got), len(props))
		}
	})
}
