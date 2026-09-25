package portfmt

import (
	"reflect"
	"testing"
)

func TestParseFilter(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		field, value, err := ParseFilter("status=Failed")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if field != "status" || value != "Failed" {
			t.Errorf("got %q=%q", field, value)
		}
	})
	t.Run("value may contain =", func(t *testing.T) {
		_, value, err := ParseFilter("query=a=b")
		if err != nil || value != "a=b" {
			t.Errorf("value = %q, err = %v", value, err)
		}
	})
	// Both halves are required: a filter narrows a result set, and an empty
	// side would silently widen it instead.
	for _, in := range []string{"status", "=Failed", "status=", "", "="} {
		t.Run("rejects "+in, func(t *testing.T) {
			if _, _, err := ParseFilter(in); err == nil {
				t.Errorf("ParseFilter(%q) accepted an incomplete filter", in)
			}
		})
	}
}

func TestParseValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want any
	}{
		{"number becomes a number", "42", 42.0},
		{"bool becomes a bool", "true", true},
		{"null becomes nil", "null", nil},
		{"object is decoded", `{"a":1}`, map[string]any{"a": 1.0}},
		{"array is decoded", `["x"]`, []any{"x"}},
		{"bare word stays a string", "active", "active"},
		// A bare timestamp is not valid JSON, so it must survive as a string
		// rather than being mangled — this is how --input ttl=... works.
		{"timestamp stays a string", "2026-06-01T09:13:26", "2026-06-01T09:13:26"},
		{"empty stays an empty string", "", ""},
		{"quoted string is unquoted", `"active"`, "active"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseValue(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestParseInputs(t *testing.T) {
	t.Run("mixed values", func(t *testing.T) {
		got, err := ParseInputs([]string{"name=svc", "count=3", `meta={"k":true}`})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]any{
			"name":  "svc",
			"count": 3.0,
			"meta":  map[string]any{"k": true},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %#v, want %#v", got, want)
		}
	})
	// Unlike a filter, an explicitly empty input is meaningful: it is how a
	// caller overrides a server-side default with nothing.
	t.Run("empty value is allowed", func(t *testing.T) {
		got, err := ParseInputs([]string{"owner="})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v, ok := got["owner"]; !ok || v != "" {
			t.Errorf("got %#v, want owner set to an empty string", got)
		}
	})
	t.Run("value may contain =", func(t *testing.T) {
		got, _ := ParseInputs([]string{"expr=a=b"})
		if got["expr"] != "a=b" {
			t.Errorf("got %#v", got)
		}
	})
	t.Run("rejects a missing =", func(t *testing.T) {
		if _, err := ParseInputs([]string{"noequals"}); err == nil {
			t.Error("accepted an input with no =")
		}
	})
	t.Run("empty slice yields an empty map, not nil", func(t *testing.T) {
		got, err := ParseInputs(nil)
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("got %#v, err %v", got, err)
		}
	})
}
