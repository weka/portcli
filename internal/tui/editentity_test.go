package tui

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	for _, tc := range []struct {
		name       string
		prev, next map[string]any
		want       map[string]any
	}{
		{
			name: "no changes",
			prev: map[string]any{"status": "OK", "count": 3.0},
			next: map[string]any{"status": "OK", "count": 3.0},
			want: map[string]any{},
		},
		{
			name: "one value changed",
			prev: map[string]any{"status": "OK", "count": 3.0},
			next: map[string]any{"status": "Failed", "count": 3.0},
			want: map[string]any{"status": "Failed"},
		},
		{
			name: "a new key is a change",
			prev: map[string]any{"status": "OK"},
			next: map[string]any{"status": "OK", "owner": "me"},
			want: map[string]any{"owner": "me"},
		},
		{
			// PATCH-with-null means different things per property type, so
			// removing a key is deliberately not treated as a deletion.
			name: "a removed key is ignored, not nulled",
			prev: map[string]any{"status": "OK", "owner": "me"},
			next: map[string]any{"status": "OK"},
			want: map[string]any{},
		},
		{
			name: "nested values compare by value",
			prev: map[string]any{"cfg": map[string]any{"a": 1.0}},
			next: map[string]any{"cfg": map[string]any{"a": 1.0}},
			want: map[string]any{},
		},
		{
			name: "a nested change is detected",
			prev: map[string]any{"cfg": map[string]any{"a": 1.0}},
			next: map[string]any{"cfg": map[string]any{"a": 2.0}},
			want: map[string]any{"cfg": map[string]any{"a": 2.0}},
		},
		{
			name: "arrays compare by value",
			prev: map[string]any{"tags": []any{"a", "b"}},
			next: map[string]any{"tags": []any{"a", "c"}},
			want: map[string]any{"tags": []any{"a", "c"}},
		},
		{
			name: "an explicit null is a change",
			prev: map[string]any{"ttl": "3h"},
			next: map[string]any{"ttl": nil},
			want: map[string]any{"ttl": nil},
		},
		{
			name: "everything from an empty starting point",
			prev: map[string]any{},
			next: map[string]any{"status": "OK"},
			want: map[string]any{"status": "OK"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Diff(tc.prev, tc.next); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestDiffSummaryIsSorted(t *testing.T) {
	got := DiffSummary(map[string]any{"zulu": 1, "alpha": 2, "mike": 3})
	if got != "alpha, mike, zulu" {
		t.Errorf("got %q, want a sorted list", got)
	}
}

func TestEditorCommand(t *testing.T) {
	t.Run("VISUAL wins over EDITOR", func(t *testing.T) {
		t.Setenv("VISUAL", "code --wait")
		t.Setenv("EDITOR", "nano")
		if got := editorCommand(); !reflect.DeepEqual(got, []string{"code", "--wait"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("EDITOR is used when VISUAL is unset", func(t *testing.T) {
		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", "nano")
		if got := editorCommand(); !reflect.DeepEqual(got, []string{"nano"}) {
			t.Errorf("got %v", got)
		}
	})
	t.Run("falls back to vi", func(t *testing.T) {
		t.Setenv("VISUAL", "")
		t.Setenv("EDITOR", "  ")
		if got := editorCommand(); !reflect.DeepEqual(got, []string{"vi"}) {
			t.Errorf("got %v", got)
		}
	})
}

// A retry prepends the parse error as comments, so they must be stripped
// before parsing and must not accumulate across attempts.
func TestStripComments(t *testing.T) {
	in := "// not valid JSON: bad\n// fix it\n{\n  \"a\": 1\n}\n"
	got := strings.TrimSpace(string(stripComments([]byte(in))))
	if got != "{\n  \"a\": 1\n}" {
		t.Errorf("got %q", got)
	}
	t.Run("a // inside a string value is preserved", func(t *testing.T) {
		// Only whole comment lines are dropped, so a URL in a value survives.
		in := "{\n  \"url\": \"https://example.com\"\n}"
		if got := string(stripComments([]byte(in))); got != in {
			t.Errorf("got %q, want it unchanged", got)
		}
	})
}

func TestSanitizeFilename(t *testing.T) {
	for in, want := range map[string]string{
		"dep-abc_123.json": "dep-abc_123.json",
		"a/b":              "a-b",
		"weird name!":      "weird-name-",
		"":                 "",
	} {
		if got := sanitizeFilename(in); got != want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
