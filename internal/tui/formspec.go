package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// FieldKind selects the widget an input gets.
type FieldKind int

const (
	FieldText FieldKind = iota
	FieldNumber
	FieldBool
	FieldSelect    // a fixed enum
	FieldEntityRef // format:entity — offers the blueprint's entities
	FieldJSON      // array or object, edited as JSON
)

// Field is one input, resolved into what a form needs to render it.
type Field struct {
	Name     string
	Label    string
	Kind     FieldKind
	Required bool
	Default  string
	Options  []string // for FieldSelect
	EntityBP string   // for FieldEntityRef
	Hint     string
	// Conditional marks an input whose visibility Port decides with a query.
	// Shown rather than hidden — see BuildFormSpec.
	Conditional bool

	Pattern   string
	MinLength *int
	MaxLength *int
	Minimum   *float64
	Maximum   *float64
}

// FormSpec is an action reduced to a fillable form.
type FormSpec struct {
	ActionID  string
	Title     string
	Blueprint string
	Operation string
	Backend   string
	Fields    []Field
}

// IsUpsert reports whether Port performs this action itself by writing an
// entity, which is the case with no run record to inspect afterwards.
func (s FormSpec) IsUpsert() bool { return s.Backend == "UPSERT_ENTITY" }

// BuildFormSpec turns an action into a form.
//
// Two decisions worth stating. Inputs whose visibility is a server-side query
// are shown, grouped and marked, not hidden: the dropped-upsert failure this
// tool exists to diagnose is *caused* by a hidden input whose default resolves
// to nothing, so hiding them would reproduce the bug rather than surface it.
// And nothing here evaluates jq — that needs both a new dependency and the
// evaluation context (.user, .entity, .blueprint) Port builds server-side and
// does not expose, so a wrong guess would be worse than an honest hint.
func BuildFormSpec(a *client.ActionDetail) FormSpec {
	inputs := a.Trigger.UserInputs
	required := make(map[string]bool, len(inputs.Required))
	for _, name := range inputs.Required {
		required[name] = true
	}

	spec := FormSpec{
		ActionID:  a.Identifier,
		Title:     a.Title,
		Blueprint: a.Trigger.BlueprintIdentifier,
		Operation: a.Trigger.Operation,
		Backend:   a.InvocationMethod.Type,
	}

	for _, name := range portfmt.InputOrder(inputs.Order, inputs.Properties) {
		spec.Fields = append(spec.Fields, buildField(name, inputs.Properties[name], required[name]))
	}

	// Unconditional inputs first: they are the ones that always apply, and a
	// stable split keeps the conditional group together at the bottom.
	sort.SliceStable(spec.Fields, func(i, j int) bool {
		return !spec.Fields[i].Conditional && spec.Fields[j].Conditional
	})
	return spec
}

func buildField(name string, in client.ActionInput, required bool) Field {
	f := Field{
		Name:        name,
		Label:       name,
		Required:    required,
		Conditional: in.ConditionalVisibility(),
		Pattern:     in.Pattern,
		MinLength:   in.MinLength,
		MaxLength:   in.MaxLength,
		Minimum:     in.Minimum,
		Maximum:     in.Maximum,
	}
	if in.Title != "" && in.Title != name {
		f.Label = fmt.Sprintf("%s (%s)", name, in.Title)
	}
	if required {
		f.Label += " *"
	}

	var hints []string
	if def, ok := in.DefaultValue(); ok {
		f.Default = portfmt.AnyToString(def)
	} else if in.DynamicDefault() {
		hints = append(hints, "default computed server-side; resolves empty on client credentials")
	}

	values, dynamicEnum := in.EnumValues()
	switch {
	case in.IsEntityRef():
		f.Kind = FieldEntityRef
		f.EntityBP = in.Blueprint
		hints = append(hints, "an entity of "+in.Blueprint)
	case dynamicEnum:
		// The choices exist but only Port can compute them.
		f.Kind = FieldText
		hints = append(hints, "choices computed server-side — type a value")
	case len(values) > 0:
		f.Kind = FieldSelect
		f.Options = make([]string, 0, len(values))
		for _, v := range values {
			f.Options = append(f.Options, portfmt.AnyToString(v))
		}
	case in.Type == "boolean":
		f.Kind = FieldBool
	case in.Type == "number" || in.Type == "integer":
		f.Kind = FieldNumber
	case in.Type == "array" || in.Type == "object":
		f.Kind = FieldJSON
		hints = append(hints, "JSON")
	default:
		f.Kind = FieldText
	}

	if len(in.DependsOn) > 0 {
		hints = append(hints, "depends on "+strings.Join(in.DependsOn, ", "))
	}
	if f.Conditional {
		hints = append(hints, "shown conditionally by Port")
	}
	if in.Description != "" {
		hints = append([]string{in.Description}, hints...)
	}
	f.Hint = strings.Join(hints, " · ")
	return f
}

// BuildPayload converts filled-in values into action inputs, collecting every
// problem rather than stopping at the first so a form can mark them all.
//
// An empty optional field is omitted rather than sent as "": Port would take
// the empty string as a real value and overwrite whatever default it would
// otherwise have applied.
func BuildPayload(spec FormSpec, values map[string]string) (map[string]any, []error) {
	props := make(map[string]any, len(values))
	var errs []error

	for _, f := range spec.Fields {
		raw, present := values[f.Name]
		raw = strings.TrimSpace(raw)

		if !present || raw == "" {
			if f.Required {
				errs = append(errs, fmt.Errorf("%s is required", f.Name))
			}
			continue
		}

		switch f.Kind {
		case FieldNumber:
			n, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s must be a number, got %q", f.Name, raw))
				continue
			}
			if f.Minimum != nil && n < *f.Minimum {
				errs = append(errs, fmt.Errorf("%s must be at least %v", f.Name, *f.Minimum))
				continue
			}
			if f.Maximum != nil && n > *f.Maximum {
				errs = append(errs, fmt.Errorf("%s must be at most %v", f.Name, *f.Maximum))
				continue
			}
			props[f.Name] = n

		case FieldBool:
			b, err := strconv.ParseBool(raw)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s must be true or false, got %q", f.Name, raw))
				continue
			}
			props[f.Name] = b

		case FieldJSON:
			var parsed any
			if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
				errs = append(errs, fmt.Errorf("%s must be valid JSON: %v", f.Name, err))
				continue
			}
			props[f.Name] = parsed

		default:
			if f.MinLength != nil && len(raw) < *f.MinLength {
				errs = append(errs, fmt.Errorf("%s must be at least %d characters", f.Name, *f.MinLength))
				continue
			}
			if f.MaxLength != nil && len(raw) > *f.MaxLength {
				errs = append(errs, fmt.Errorf("%s must be at most %d characters", f.Name, *f.MaxLength))
				continue
			}
			if f.Pattern != "" {
				ok, err := matchPattern(f.Pattern, raw)
				if err == nil && !ok {
					errs = append(errs, fmt.Errorf("%s must match %s", f.Name, f.Pattern))
					continue
				}
				// An unparsable pattern is Port's problem, not the user's:
				// let the value through and let the API reject it.
			}
			props[f.Name] = raw
		}
	}

	if len(errs) > 0 {
		return nil, errs
	}
	return props, nil
}

// matchPattern applies a JSON Schema pattern. Go's regexp is RE2, which does
// not accept every ECMA construct, so an unparsable pattern is reported as
// such rather than treated as a failed match.
func matchPattern(pattern, value string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(value), nil
}
