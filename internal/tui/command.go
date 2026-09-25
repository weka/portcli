package tui

import (
	"sort"
	"strings"
)

// Command is a parsed palette entry: a resource kind and the arguments that
// narrow it, e.g. ":entities deployment status=Failed".
type Command struct {
	Kind   string
	Args   []string
	Filter string // the trailing field=value, if one was given
}

// ParseCommand splits palette input. The leading ":" is optional so the same
// parser serves both the prompt and a restored view string.
//
// A trailing field=value argument is lifted into Filter: a server-side entity
// filter is a different request from the client-side "/" filter, and keeping
// them apart here is what stops the two being conflated.
func ParseCommand(s string) (Command, bool) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), ":"))
	if s == "" {
		return Command{}, false
	}

	fields := strings.Fields(s)
	cmd := Command{Kind: strings.ToLower(fields[0])}
	for _, f := range fields[1:] {
		// Only the last field=value wins as the filter; earlier ones stay
		// positional, since a resource may legitimately take one.
		if strings.Contains(f, "=") {
			cmd.Filter = f
			continue
		}
		cmd.Args = append(cmd.Args, f)
	}
	return cmd, true
}

// String renders a Command back into palette form, which is what gets
// persisted as the last view.
func (c Command) String() string {
	parts := append([]string{c.Kind}, c.Args...)
	if c.Filter != "" {
		parts = append(parts, c.Filter)
	}
	return strings.Join(parts, " ")
}

// Candidates completes palette input. While the kind is still being typed it
// offers resource names; once a kind is settled it offers that kind's
// arguments from what previous views already loaded, so completion costs no
// requests.
func Candidates(input string, caches map[string][]string) []string {
	// Only the leading whitespace and colon are noise. A *trailing* space is
	// significant — it is what distinguishes "finished naming the resource,
	// now starting an argument" from "still typing the resource name" — so
	// TrimSpace would destroy the one signal this function runs on.
	trimmed := strings.TrimPrefix(strings.TrimLeft(input, " \t"), ":")
	if strings.TrimSpace(trimmed) == "" {
		return Kinds()
	}

	// No space yet means the user is still naming the resource.
	if !strings.Contains(trimmed, " ") {
		return withPrefix(Names(), strings.ToLower(trimmed))
	}

	fields := strings.Fields(trimmed)
	kind := strings.ToLower(fields[0])
	reg, ok := registry[kind]
	if !ok {
		return nil
	}

	// A trailing space means the last argument is finished and a new one is
	// starting; without one, the last token is still being typed.
	settled, partial := fields[1:], ""
	if !strings.HasSuffix(trimmed, " ") && len(fields) > 1 {
		settled, partial = fields[1:len(fields)-1], fields[len(fields)-1]
	}
	prefix := strings.Join(append([]string{kind}, settled...), " ")

	out := make([]string, 0, 8)
	for _, arg := range withPrefix(caches[reg.kind], partial) {
		out = append(out, prefix+" "+arg)
	}
	return out
}

// withPrefix filters candidates case-insensitively, preferring a real prefix
// match but falling back to a substring so a partial mid-name still finds it.
func withPrefix(candidates []string, partial string) []string {
	if partial == "" {
		return candidates
	}
	lower := strings.ToLower(partial)
	var prefixed, contained []string
	for _, c := range candidates {
		lc := strings.ToLower(c)
		switch {
		case strings.HasPrefix(lc, lower):
			prefixed = append(prefixed, c)
		case strings.Contains(lc, lower):
			contained = append(contained, c)
		}
	}
	sort.Strings(prefixed)
	sort.Strings(contained)
	return append(prefixed, contained...)
}
