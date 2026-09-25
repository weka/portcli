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
	return fmt.Sprintf("[darkcyan::b]%-*s[white::-]%s", width, name+":", value)
}

// hintLines renders the key hints beside the context panel: the operations the
// current view offers, then the keys that always work.
func hintLines(ops []Op) []string {
	var view []string
	for _, op := range ops {
		view = append(view, fmt.Sprintf("[dimgray]<[white]%s[dimgray]>[-] %s", op.Label(), op.Name))
	}

	global := []string{
		"[dimgray]<[white]:[dimgray]>[-] command   [dimgray]<[white]/[dimgray]>[-] filter",
		"[dimgray]<[white]?[dimgray]>[-] help      [dimgray]<[white]ctrl-r[dimgray]>[-] refresh",
		"[dimgray]<[white]esc[dimgray]>[-] back    [dimgray]<[white]q[dimgray]>[-] quit",
	}

	// Two columns' worth of view hints, then the globals, is about what fits
	// beside the context panel without wrapping on an 80-column terminal.
	if len(view) > 3 {
		view = append(view[:3], fmt.Sprintf("[dimgray]… +%d more (press ?)", len(view)-3))
	}
	return append(view, global...)
}
