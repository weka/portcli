package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/client"
)

// Column describes one table column.
type Column struct {
	Name string // heading, already in display form ("IDENTIFIER")
	Wide bool   // hidden until the user asks for wide columns
}

// Row is one table row.
type Row struct {
	// ID is a stable key. Selection and marks are restored by ID after a
	// refresh, so a row that moved does not take the cursor with it.
	ID    string
	Cells []string // one per Column, in order
	Obj   any      // the typed client value, for describe and operations
}

// Op is a keyed operation offered by a resource.
type Op struct {
	Rune      rune      // 0 when Key is set instead
	Key       tcell.Key // for named keys such as Enter
	Name      string    // shown in the header hints and the help overlay
	Dangerous bool      // must be confirmed before running
	Run       func(a *App, rows []Row) error
}

// Label renders the key as a user sees it.
func (o Op) Label() string {
	switch {
	case o.Rune != 0:
		return string(o.Rune)
	case o.Key == tcell.KeyEnter:
		return "enter"
	case o.Key == tcell.KeyCtrlD:
		return "ctrl-d"
	default:
		return tcell.KeyNames[o.Key]
	}
}

// Matches reports whether ev triggers this operation.
func (o Op) Matches(ev *tcell.EventKey) bool {
	if o.Rune != 0 {
		return ev.Key() == tcell.KeyRune && ev.Rune() == o.Rune
	}
	return o.Key != 0 && ev.Key() == o.Key
}

// Resource is a listable Port collection. It is pure data plus presentation
// metadata: it never touches tview and never decides what the UI does, so it
// can be tested without a screen.
type Resource interface {
	// Kind is the registered name, e.g. "blueprints".
	Kind() string
	// ID identifies this resource including its arguments, e.g.
	// "entities/deployment". It keys the view stack, so two views of the same
	// kind over different arguments stay distinct.
	ID() string
	// Title is the heading shown in the breadcrumb.
	Title() string
	Columns() []Column
	List(ctx context.Context, c *client.Client) ([]Row, error)
	Ops() []Op
}

// factory builds a resource from the arguments typed after its name in the
// command palette.
type factory func(args []string) (Resource, error)

type registration struct {
	kind    string
	aliases []string
	new     factory
}

var registry = map[string]*registration{}

// Register makes a resource reachable from the command palette. Called from
// each resource file's init, so adding a resource is one self-contained file.
func Register(kind string, aliases []string, new factory) {
	reg := &registration{kind: kind, aliases: aliases, new: new}
	registry[kind] = reg
	for _, a := range aliases {
		registry[a] = reg
	}
}

// Resolve builds the resource named by kind, which may be an alias.
func Resolve(kind string, args []string) (Resource, error) {
	reg, ok := registry[strings.ToLower(kind)]
	if !ok {
		return nil, fmt.Errorf("unknown resource %q (try %s)", kind, strings.Join(Kinds(), ", "))
	}
	return reg.new(args)
}

// Kinds lists the canonical resource names, sorted.
func Kinds() []string {
	seen := map[string]bool{}
	var out []string
	for _, reg := range registry {
		if !seen[reg.kind] {
			seen[reg.kind] = true
			out = append(out, reg.kind)
		}
	}
	sort.Strings(out)
	return out
}

// Names lists every accepted spelling, canonical names and aliases, sorted —
// the candidate set for palette completion.
func Names() []string {
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
