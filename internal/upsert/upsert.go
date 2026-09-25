// Package upsert compensates for Port keeping no run record for UPSERT_ENTITY
// actions. Their run ids 404 the moment the action finishes, whether or not
// the entity was written, so the entity itself is the only evidence that the
// action did anything — and a rejected upsert is otherwise indistinguishable
// from a successful one.
package upsert

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// Request describes a run that was just triggered. Identifier is the caller's
// explicit choice of entity id — the CLI's --id flag, or a TUI form field. It
// is passed rather than read from package state so both callers can use this
// without inheriting the other's flags.
type Request struct {
	ActionID   string
	RunID      string
	Identifier string
	RunProps   map[string]any
}

// inputRefRe matches "{{ .inputs.NAME }}", the shape Port uses to copy an
// action input straight into an upserted entity.
var inputRefRe = regexp.MustCompile(`^\{\{\s*\.inputs\.([A-Za-z0-9_]+)\s*\}\}$`)

// InputRef reports the input name a mapping template reads, if it reads
// exactly one.
func InputRef(tmpl string) (string, bool) {
	m := inputRefRe.FindStringSubmatch(strings.TrimSpace(tmpl))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// Verify confirms the entity an UPSERT_ENTITY action was meant to write
// actually exists, turning a silently dropped upsert into a real error. It
// returns (nil, nil) for any other action type, or when the target entity
// cannot be determined — in those cases there is nothing to check and the
// normal run-status flow applies.
func Verify(ctx context.Context, c *client.Client, req Request) (*client.Entity, error) {
	// Report a failed lookup rather than reading it as "not an upsert".
	// Whether verification applies is exactly what this call answers, so
	// treating an error as "nothing to check" turns any transient failure into
	// a silent pass — the phantom success this package exists to prevent.
	action, err := c.GetAction(ctx, req.ActionID)
	if err != nil {
		return nil, fmt.Errorf("cannot tell whether %s is an UPSERT_ENTITY action, so its result is unverified: %w", req.ActionID, err)
	}
	if action.InvocationMethod.Type != "UPSERT_ENTITY" {
		return nil, nil
	}

	blueprint := action.InvocationMethod.BlueprintIdentifier
	target := Target(action, req.RunProps, req.Identifier)
	if blueprint == "" || target == "" {
		return nil, nil
	}

	// The upsert is not synchronous with the response — it lands a few hundred
	// milliseconds later — so allow a short grace period before concluding it
	// never happened.
	entity, err := c.PollEntity(ctx, blueprint, target, 10*time.Second, 250*time.Millisecond,
		func(_ *client.Entity, fetchErr error) (bool, error) { return fetchErr == nil, nil })
	if err == nil {
		return entity, nil
	}

	var detail string
	if unresolved := UnresolvedRequired(ctx, c, action, req.RunProps); len(unresolved) > 0 {
		detail = fmt.Sprintf("\n  required %s properties that did not resolve:\n    %s",
			blueprint, strings.Join(unresolved, "\n    "))
	}
	return nil, fmt.Errorf(
		"action %s did not create entity %s/%s%s\n  (Port stores no run record for UPSERT_ENTITY actions, so run %s cannot be inspected)",
		req.ActionID, blueprint, target, detail, req.RunID)
}

// Target resolves the identifier an UPSERT_ENTITY action writes to. override
// wins because Port applies it as the created entity's identifier; otherwise
// the mapping is normally a template over one of the inputs.
func Target(action *client.ActionDetail, runProps map[string]any, override string) string {
	if override != "" {
		return override
	}
	tmpl := strings.TrimSpace(action.InvocationMethod.Mapping.Identifier)
	if name, ok := InputRef(tmpl); ok {
		return portfmt.AnyToString(runProps[name])
	}
	if !strings.Contains(tmpl, "{{") {
		return tmpl // a static identifier
	}
	return "" // a template we cannot evaluate (jq, concatenation, …)
}

// UnresolvedRequired names blueprint-required properties whose mapped value
// came out empty. A hidden input defaulted from a jqQuery such as .user.email
// is the usual cause: it resolves to nothing when the action runs on client
// credentials rather than as a real user, and the upsert is then rejected.
func UnresolvedRequired(ctx context.Context, c *client.Client, action *client.ActionDetail, runProps map[string]any) []string {
	bp, err := c.GetBlueprint(ctx, action.InvocationMethod.BlueprintIdentifier)
	if err != nil {
		return nil
	}
	mapping := action.InvocationMethod.Mapping.Properties

	var unresolved []string
	for _, name := range bp.Schema.Required {
		tmpl, mapped := mapping[name]
		if !mapped {
			unresolved = append(unresolved, name+" — not set by the action mapping")
			continue
		}
		input, ok := InputRef(portfmt.AnyToString(tmpl))
		if !ok {
			continue // a literal or expression we cannot evaluate; assume it resolved
		}
		if portfmt.AnyToString(runProps[input]) == "" {
			unresolved = append(unresolved,
				fmt.Sprintf("%s — from input %q, which resolved empty (pass --input %s=...)", name, input, input))
		}
	}
	sort.Strings(unresolved)
	return unresolved
}

// ExplainMissingRun annotates a 404 from a run lookup. Port discards the run
// record for UPSERT_ENTITY actions as soon as they finish, so those ids never
// resolve whether the action succeeded or not; unannotated, the 404 reads as
// if the run were lost.
func ExplainMissingRun(err error) error {
	if !client.IsNotFound(err) {
		return err
	}
	return fmt.Errorf("%w\n  (Port keeps no run record for UPSERT_ENTITY actions, so their run ids never resolve — inspect the target entity instead)", err)
}
