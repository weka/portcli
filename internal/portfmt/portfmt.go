// Package portfmt renders Port API values as display strings and parses the
// command-line spellings of those values back. It holds no state and makes no
// network calls, so both the CLI and the TUI can share it without either one
// importing the other.
package portfmt

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/weka/portcli/internal/client"
)

// AnyToString renders a decoded JSON value as a plain string: strings as
// themselves, everything else re-encoded. A nil value is empty rather than
// "null", since it reaches here to fill a table cell.
func AnyToString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	out, _ := json.Marshal(v)
	return string(out)
}

// PropStr renders one property of an entity. Indexing a nil or missing key
// yields nil, which AnyToString already renders as empty.
func PropStr(props map[string]any, key string) string {
	return AnyToString(props[key])
}

// CamelToSnake inserts underscores before capitals, turning a property name
// into a column heading (`createdAt` becomes `CREATED_AT` once upper-cased).
func CamelToSnake(s string) string {
	var result []byte
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '_')
		}
		result = append(result, byte(r))
	}
	return string(result)
}

// Date trims an ISO timestamp to just the date portion.
func Date(s string) string {
	if t := strings.Index(s, "T"); t > 0 {
		return s[:t]
	}
	return s
}

// DateTime trims an ISO timestamp to date and time, dropping sub-seconds.
func DateTime(s string) string {
	if idx := strings.Index(s, "."); idx > 0 {
		s = s[:idx]
	}
	return strings.Replace(s, "T", " ", 1)
}

// EntityCol returns one column's value for an entity, resolving the few
// top-level fields that are not properties before falling back to the
// property bag.
func EntityCol(e client.EntitySummary, col string) string {
	switch strings.ToLower(col) {
	case "createdat":
		return Date(e.CreatedAt)
	case "createdby":
		return e.CreatedBy
	default:
		return PropStr(e.Properties, col)
	}
}

// EnumPreview joins enum values for a table cell, truncating long lists.
func EnumPreview(enum []any) string {
	if len(enum) == 0 {
		return ""
	}
	vals := make([]string, len(enum))
	for i, v := range enum {
		vals[i] = AnyToString(v)
	}
	const max = 6
	if len(vals) > max {
		return fmt.Sprintf("%s,… (+%d more)", strings.Join(vals[:max], ","), len(vals)-max)
	}
	return strings.Join(vals, ",")
}

// InputOrder returns input names in the action's declared order, appending any
// the action did not list (sorted) so nothing is silently dropped.
func InputOrder(order []string, props map[string]client.ActionInput) []string {
	seen := make(map[string]bool, len(props))
	names := make([]string, 0, len(props))
	for _, name := range order {
		if _, ok := props[name]; ok && !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	rest := make([]string, 0, len(props))
	for name := range props {
		if !seen[name] {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	return append(names, rest...)
}
