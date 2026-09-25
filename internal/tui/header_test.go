package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestMaskID(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"abcd1234efgh", "abcd…efgh"},
		{"short", "•••••"},
		{"123456789", "•••••••••"}, // exactly 9: masking entirely still hides it
		{"", ""},
	} {
		if got := maskID(tc.in); got != tc.want {
			t.Errorf("maskID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestHeaderLines(t *testing.T) {
	base := headerInfo{
		BaseURL:    "https://api.getport.io",
		ClientID:   "abcd1234efgh",
		AuthSource: "env PORT_CLIENT_ID",
		Version:    "v1.2.3",
		Resource:   "entities/deployment",
		Interval:   30 * time.Second,
		NextIn:     12 * time.Second,
		Shown:      184,
		Total:      1204,
	}

	t.Run("reports the essentials", func(t *testing.T) {
		out := strings.Join(base.headerLines(), "\n")
		for _, want := range []string{
			"https://api.getport.io",
			"env PORT_CLIENT_ID", // which credentials are in play is load-bearing
			"abcd…efgh",
			"v1.2.3",
			"entities/deployment",
			"next in 12s",
			"184 shown / 1204 total",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q in:\n%s", want, out)
			}
		}
	})

	t.Run("no row count when nothing is filtered out", func(t *testing.T) {
		info := base
		info.Total = info.Shown
		out := strings.Join(info.headerLines(), "\n")
		if strings.Contains(out, "shown /") {
			t.Errorf("should not mention filtering:\n%s", out)
		}
	})

	t.Run("marks are counted", func(t *testing.T) {
		info := base
		info.Marked = 3
		if !strings.Contains(strings.Join(info.headerLines(), "\n"), "3 marked") {
			t.Error("marked count missing")
		}
	})

	// A failed background refresh must be visible in the header, not only in a
	// flash that has already timed out — otherwise stale rows look current.
	t.Run("a failed refresh replaces the countdown", func(t *testing.T) {
		info := base
		info.RefreshFail = "API error (status 500)"
		out := strings.Join(info.headerLines(), "\n")
		if !strings.Contains(out, "API error (status 500)") {
			t.Errorf("failure not reported:\n%s", out)
		}
		if strings.Contains(out, "next in") {
			t.Errorf("countdown should be replaced by the failure:\n%s", out)
		}
	})
}

func TestHintLinesTruncateLongOpLists(t *testing.T) {
	ops := []Op{
		{Rune: 'd', Name: "Describe"},
		{Rune: 'e', Name: "Edit"},
		{Rune: 'l', Name: "Logs"},
		{Rune: 'r', Name: "Run"},
		{Key: tcell.KeyCtrlD, Name: "Delete"},
	}
	out := strings.Join(hintLines(ops), "\n")
	if !strings.Contains(out, "+2 more") {
		t.Errorf("long op lists should be truncated with a pointer to help:\n%s", out)
	}
	// The always-available keys must survive truncation.
	for _, want := range []string{"command", "filter", "help", "quit"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing global hint %q in:\n%s", want, out)
		}
	}
}

func TestOpLabel(t *testing.T) {
	for _, tc := range []struct {
		op   Op
		want string
	}{
		{Op{Rune: 'd'}, "d"},
		{Op{Key: tcell.KeyEnter}, "enter"},
		{Op{Key: tcell.KeyCtrlD}, "ctrl-d"},
	} {
		if got := tc.op.Label(); got != tc.want {
			t.Errorf("Label() = %q, want %q", got, tc.want)
		}
	}
}

// Every registered operation must avoid the keys tview.Table handles itself,
// unless it is one we deliberately steal. This is the cheap guard against the
// class of bug that makes a TUI feel broken for no visible reason.
func TestNoOpShadowsAReservedTableKey(t *testing.T) {
	reserved := map[rune]string{
		'j': "row down", 'k': "row up",
		'g': "first row", 'G': "last row",
	}
	for _, kind := range Kinds() {
		res, err := Resolve(kind, []string{"placeholder"})
		if err != nil {
			continue // a resource needing real arguments; covered by its own test
		}
		for _, op := range res.Ops() {
			if op.Rune == 0 {
				continue
			}
			if what, clash := reserved[op.Rune]; clash && !stolenKeys[op.Rune] {
				t.Errorf("%s binds %q to %q, which tview.Table uses for %s",
					kind, op.Rune, op.Name, what)
			}
		}
	}
}
