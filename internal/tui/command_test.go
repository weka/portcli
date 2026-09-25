package tui

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     Command
		ok       bool
	}{
		{"bare kind", "blueprints", Command{Kind: "blueprints"}, true},
		{"leading colon", ":blueprints", Command{Kind: "blueprints"}, true},
		{"colon and spaces", "  :  blueprints  ", Command{Kind: "blueprints"}, true},
		{"kind is lowercased", "BluePrints", Command{Kind: "blueprints"}, true},
		{"one argument", "entities deployment", Command{Kind: "entities", Args: []string{"deployment"}}, true},
		{
			"argument and filter",
			"entities deployment status=Failed",
			Command{Kind: "entities", Args: []string{"deployment"}, Filter: "status=Failed"},
			true,
		},
		{
			// A filter value may itself contain "=", so only the split matters.
			"filter value containing equals",
			"entities bp query=a=b",
			Command{Kind: "entities", Args: []string{"bp"}, Filter: "query=a=b"},
			true,
		},
		{"empty", "", Command{}, false},
		{"just a colon", ":", Command{}, false},
		{"only whitespace", "   ", Command{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseCommand(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// A Command has to survive being written to the state file and read back, or
// "remember the last view" silently reopens something else.
func TestCommandStringRoundTrips(t *testing.T) {
	for _, in := range []string{
		"blueprints",
		"entities deployment",
		"entities deployment status=Failed",
	} {
		t.Run(in, func(t *testing.T) {
			first, ok := ParseCommand(in)
			if !ok {
				t.Fatalf("ParseCommand(%q) failed", in)
			}
			if got := first.String(); got != in {
				t.Errorf("String() = %q, want %q", got, in)
			}
			second, _ := ParseCommand(first.String())
			if !reflect.DeepEqual(first, second) {
				t.Errorf("reparse differs: %+v vs %+v", first, second)
			}
		})
	}
}

func TestCandidates(t *testing.T) {
	caches := map[string][]string{
		"blueprints": {"deployment", "service", "test_result"},
	}

	t.Run("empty input offers every kind", func(t *testing.T) {
		got := Candidates("", caches)
		if !reflect.DeepEqual(got, Kinds()) {
			t.Errorf("got %v, want %v", got, Kinds())
		}
	})

	t.Run("partial kind matches by prefix", func(t *testing.T) {
		for _, c := range Candidates("blue", caches) {
			if !strings.HasPrefix(c, "blue") {
				t.Errorf("candidate %q does not start with the input", c)
			}
		}
	})

	t.Run("an alias is offered too", func(t *testing.T) {
		var found bool
		for _, c := range Candidates("b", caches) {
			if c == "bp" {
				found = true
			}
		}
		if !found {
			t.Errorf("alias bp missing from %v", Candidates("b", caches))
		}
	})

	t.Run("after a kind and a space, arguments are offered", func(t *testing.T) {
		got := Candidates("blueprints ", caches)
		want := []string{"blueprints deployment", "blueprints service", "blueprints test_result"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("a partial argument narrows the list", func(t *testing.T) {
		got := Candidates("blueprints serv", caches)
		want := []string{"blueprints service"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("an unknown kind offers nothing", func(t *testing.T) {
		if got := Candidates("nonsense ", caches); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
}

func TestWithPrefixPrefersPrefixOverSubstring(t *testing.T) {
	// A prefix match is what the user most likely meant, but a substring hit
	// still beats offering nothing when they typed the middle of a name.
	got := withPrefix([]string{"my-deployment", "deployment", "deployment-two"}, "deploy")
	want := []string{"deployment", "deployment-two", "my-deployment"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
