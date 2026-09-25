package tui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// headerInfo is everything the context panel reports. Split out from the
// widget so the formatting is testable without a screen.
type headerInfo struct {
	BaseURL     string
	ClientID    string
	AuthSource  string
	Version     string
	Resource    string
	Interval    time.Duration
	NextIn      time.Duration
	Refreshing  bool
	RefreshFail string // non-empty when the last background refresh failed
	Shown       int
	Total       int
	Marked      int
}

// authSource describes where the credentials came from, resolved the same way
// config.Load resolves them.
//
// This is load-bearing rather than decorative: an UPSERT_ENTITY action whose
// input defaults from a jqQuery over the calling user resolves to nothing
// under client credentials, and someone staring at a dropped upsert needs to
// see which credentials are actually in play.
func authSource() string {
	if os.Getenv("PORT_CLIENT_ID") != "" {
		return "env PORT_CLIENT_ID"
	}
	if home, err := os.UserHomeDir(); err == nil {
		if _, err := os.Stat(home + "/.portcli/config.json"); err == nil {
			return "~/.portcli/config.json"
		}
	}
	return "unknown"
}

// maskID shows enough of the client id to tell two credentials apart without
// putting a full secret-adjacent value on a screen that may be shared. The
// client secret is never rendered anywhere.
func maskID(id string) string {
	if len(id) <= 9 {
		return strings.Repeat("•", len(id))
	}
	return id[:4] + "…" + id[len(id)-4:]
}

// headerLines renders the context panel. Returned as lines so a test can
// assert on content rather than on a drawn frame.
func (h headerInfo) headerLines() []string {
	refresh := "off"
	if h.Interval > 0 {
		refresh = fmt.Sprintf("%s · next in %s", h.Interval, h.NextIn.Round(time.Second))
		if h.Refreshing {
			refresh += " · ⟳"
		}
	}
	if h.RefreshFail != "" {
		refresh = "⚠ " + h.RefreshFail
	}

	rows := fmt.Sprintf("%d", h.Shown)
	if h.Total != h.Shown {
		rows += fmt.Sprintf(" shown / %d total", h.Total)
	}
	if h.Marked > 0 {
		rows += fmt.Sprintf(" · %d marked", h.Marked)
	}

	return []string{
		fieldLine("Base URL", h.BaseURL, 11),
		fieldLine("Auth", h.AuthSource, 11),
		fieldLine("Client ID", maskID(h.ClientID), 11),
		fieldLine("Version", h.Version, 11),
		fieldLine("Resource", h.Resource, 11),
		fieldLine("Refresh", refresh, 11),
		fieldLine("Rows", rows, 11),
	}
}

// fieldLine renders a "Name: value" row of the label-and-value style used by
// the context panel and the status cards. width aligns the labels within one
// block; blocks differ, so it is a parameter rather than a constant.
func fieldLine(name, value string, width int) string {
	return fmt.Sprintf("[%s::b]%-*s[%s::-]%s", tagLabel, width, name+":", tagValue, value)
}

// Widths of the header's fixed blocks.
const (
	ctxPanelWidth = 38
	logoWidth     = 27
)

// hintRows is how many rows of key hints the header has room for. The context
// panel is the tallest block, and the legend reads across into it.
const hintRows = 6

// hint renders one "<key> description" pair, padded so a column lines up.
// The visible width has to be computed from the plain text, since the colour
// tags are markup rather than characters on screen.
func hint(key, desc string, width int) string {
	plain := fmt.Sprintf("<%s> %s", key, desc)
	pad := width - len([]rune(plain))
	if pad < 0 {
		pad = 0
	}
	// k9s draws the angle brackets in the key's own colour rather than dimming
	// them, which makes each chord read as one token.
	return fmt.Sprintf("[%s::b]<%s>[%s::-] %s%s",
		tagKey, key, tagHint, desc, strings.Repeat(" ", pad))
}

// hintLines lays the key legend out in columns, filling top to bottom like
// k9s does, so the header stays a fixed height however many operations the
// current view offers.
func hintLines(ops []Op) []string {
	type pair struct{ key, desc string }
	pairs := make([]pair, 0, len(ops)+8)
	for _, op := range ops {
		pairs = append(pairs, pair{op.Label(), op.Name})
	}
	// The always-available keys come last so a view's own operations lead.
	for _, g := range []pair{
		{":", "command"}, {"/", "filter"}, {"?", "help"},
		{"ctrl-r", "refresh"}, {"space", "mark"}, {"esc", "back"}, {"q", "quit"},
	} {
		pairs = append(pairs, g)
	}

	// Column width from the widest entry in each column, so short columns do
	// not get padded out to the width of a long one.
	columns := (len(pairs) + hintRows - 1) / hintRows
	widths := make([]int, columns)
	for i, p := range pairs {
		c := i / hintRows
		if w := len(p.key) + len(p.desc) + 3; w > widths[c] {
			widths[c] = w
		}
	}

	lines := make([]string, hintRows)
	for i, p := range pairs {
		row, col := i%hintRows, i/hintRows
		lines[row] += hint(p.key, p.desc, widths[col]+2)
	}
	return lines
}

// logo returns the banner, coloured.
func logo() string {
	out := make([]string, len(logoLines))
	for i, l := range logoLines {
		out[i] = "[" + tagLabel + "::b]" + l
	}
	return strings.Join(out, "\n")
}
