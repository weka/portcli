package portfmt

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseFilter splits a "field=value" filter into its parts. Both halves must be
// present: a filter is a narrowing instruction, and an empty side would widen
// it to everything instead, which is rarely what was meant.
func ParseFilter(filter string) (field, value string, err error) {
	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid filter format, expected field=value")
	}
	return parts[0], parts[1], nil
}

// ParseValue interprets a command-line value as JSON where it parses, so
// numbers, booleans, objects and arrays survive, and as a plain string
// otherwise.
func ParseValue(raw string) any {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return raw
}

// ParseInputs turns repeated "key=value" action inputs into a property map.
// Unlike ParseFilter an empty value is allowed: passing an input explicitly
// empty is how a caller overrides a default.
func ParseInputs(inputs []string) (map[string]any, error) {
	props := make(map[string]any, len(inputs))
	for _, input := range inputs {
		key, val, ok := strings.Cut(input, "=")
		if !ok {
			return nil, fmt.Errorf("invalid input format %q, expected key=value", input)
		}
		props[key] = ParseValue(val)
	}
	return props, nil
}
