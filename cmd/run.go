package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var (
	inputFlags   []string
	wait         bool
	pollSeconds  int
	timeoutSecs  int
	runAs        string
	entityID     string
	identifier string
	runJSON      bool
)

var actionRunCmd = &cobra.Command{
	Use:   "run <action-identifier>",
	Short: "Execute a self-service action",
	Long: `Execute a Port self-service action and optionally wait for completion.

Examples:
  portcli action run create_microservice --input name=my-service --input language=go
  portcli action run deploy_service --input service=checkout --wait
  portcli action run deploy_service --input service=checkout --wait --timeout 300`,
	Args: cobra.ExactArgs(1),
	RunE: runAction,
}

func init() {
	actionRunCmd.Flags().StringArrayVarP(&inputFlags, "input", "i", nil, "Action input as key=value (can be repeated)")
	actionRunCmd.Flags().BoolVarP(&wait, "wait", "w", false, "Wait for the action run to complete")
	actionRunCmd.Flags().IntVar(&pollSeconds, "poll", 2, "Poll interval in seconds when waiting")
	actionRunCmd.Flags().IntVar(&timeoutSecs, "timeout", 120, "Timeout in seconds when waiting")
	actionRunCmd.Flags().StringVar(&runAs, "run-as", "", "Execute the action on behalf of this user email")
	actionRunCmd.Flags().StringVar(&entityID, "entity", "", "Target entity identifier (for day-2 actions on existing entities)")
	actionRunCmd.Flags().StringVar(&identifier, "id", "", "Custom identifier for the created entity")
	actionRunCmd.Flags().BoolVar(&runJSON, "json", false, "Emit a single machine-readable JSON object (run_id, status, identifier, linked_entities) instead of the full run dump")
}

func runAction(cmd *cobra.Command, args []string) error {
	actionID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	properties, err := parseInputs(inputFlags)
	if err != nil {
		return err
	}

	c := client.New(cfg)

	fmt.Fprintf(os.Stderr, "Executing action: %s\n", actionID)
	result, err := c.ExecuteAction(actionID, properties, runAs, entityID, identifier)
	if err != nil {
		return fmt.Errorf("failed to execute action: %w", err)
	}

	runID := result.Run.ID
	fmt.Fprintf(os.Stderr, "Run created: %s (status: %s)\n", runID, result.Run.Status)

	// Port keeps no run record for UPSERT_ENTITY actions: the run id 404s
	// immediately whether or not the upsert succeeded, so neither --wait nor
	// `action status` can ever report the outcome. The entity is the only
	// evidence, so confirm it landed instead of reporting a phantom success.
	upserted, err := verifyUpsert(c, actionID, runID, result.Run.Properties)
	if err != nil {
		return err
	}
	if upserted != nil {
		result.Run.Status = "SUCCESS"
		result.Run.Link, _ = json.Marshal([]client.EntityLink{{
			Blueprint:  upserted.Entity.Blueprint,
			Identifier: upserted.Entity.Identifier,
		}})
	}

	if wait && result.Run.Status == "IN_PROGRESS" {
		fmt.Fprintf(os.Stderr, "Waiting for completion (timeout: %ds)...\n", timeoutSecs)
		result, err = c.WaitForRun(
			runID,
			time.Duration(pollSeconds)*time.Second,
			time.Duration(timeoutSecs)*time.Second,
		)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Run completed: %s\n", result.Run.Status)
	}

	if runJSON {
		emitRunJSON(result)
	} else {
		out, _ := json.MarshalIndent(result.Run, "", "  ")
		fmt.Println(string(out))
		printLinkedEntities(c, result)
	}

	// With --wait we know the terminal status, so reflect a failed run in the exit
	// code instead of exiting 0 regardless. Without --wait the run is still
	// IN_PROGRESS and there is nothing to fail on yet.
	if wait && result.Run.Status == "FAILURE" {
		return fmt.Errorf("run %s finished with status FAILURE", runID)
	}
	return nil
}

// explainMissingRun annotates a 404 from a run lookup. Port discards the run
// record for UPSERT_ENTITY actions as soon as they finish, so those ids never
// resolve whether the action succeeded or not; unannotated, the 404 reads as
// if the run were lost.
func explainMissingRun(err error) error {
	if !client.IsNotFound(err) {
		return err
	}
	return fmt.Errorf("%w\n  (Port keeps no run record for UPSERT_ENTITY actions, so their run ids never resolve — inspect the target entity instead)", err)
}

// inputRefRe matches the "{{ .inputs.NAME }}" mapping form, the shape Port
// uses to copy an action input straight into an upserted entity.
var inputRefRe = regexp.MustCompile(`^\{\{\s*\.inputs\.([A-Za-z0-9_]+)\s*\}\}$`)

// inputRef reports the input name a mapping template reads, if it reads exactly one.
func inputRef(tmpl string) (string, bool) {
	m := inputRefRe.FindStringSubmatch(strings.TrimSpace(tmpl))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// verifyUpsert confirms the entity an UPSERT_ENTITY action was meant to write
// actually exists, turning a silently dropped upsert into a real error. It
// returns (nil, nil) for any other action type, or when the target entity
// cannot be determined — in those cases there is nothing to check and the
// normal run-status flow applies.
func verifyUpsert(c *client.Client, actionID, runID string, runProps map[string]any) (*client.Entity, error) {
	// Report a failed lookup rather than reading it as "not an upsert". Whether
	// verification applies is exactly what this call answers, so treating an
	// error as "nothing to check" turns any transient failure into a silent
	// pass — the phantom success this whole function exists to prevent.
	action, err := c.GetAction(actionID)
	if err != nil {
		return nil, fmt.Errorf("cannot tell whether %s is an UPSERT_ENTITY action, so its result is unverified: %w", actionID, err)
	}
	if action.InvocationMethod.Type != "UPSERT_ENTITY" {
		return nil, nil
	}

	blueprint := action.InvocationMethod.BlueprintIdentifier
	target := upsertTarget(action, runProps)
	if blueprint == "" || target == "" {
		return nil, nil
	}

	// The upsert is not synchronous with the response — it lands a few hundred
	// milliseconds later — so allow a short grace period before concluding it
	// never happened.
	entity, err := c.PollEntity(blueprint, target, 10*time.Second, 250*time.Millisecond,
		func(_ *client.Entity, fetchErr error) (bool, error) { return fetchErr == nil, nil })
	if err == nil {
		return entity, nil
	}

	var detail string
	if unresolved := unresolvedRequired(c, action, runProps); len(unresolved) > 0 {
		detail = fmt.Sprintf("\n  required %s properties that did not resolve:\n    %s",
			blueprint, strings.Join(unresolved, "\n    "))
	}
	return nil, fmt.Errorf(
		"action %s did not create entity %s/%s%s\n  (Port stores no run record for UPSERT_ENTITY actions, so run %s cannot be inspected)",
		actionID, blueprint, target, detail, runID)
}

// upsertTarget resolves the identifier an UPSERT_ENTITY action writes to.
// --id wins because Port applies it as the created entity's identifier;
// otherwise the mapping is normally a template over one of the inputs.
func upsertTarget(action *client.ActionDetail, runProps map[string]any) string {
	if identifier != "" {
		return identifier
	}
	tmpl := strings.TrimSpace(action.InvocationMethod.Mapping.Identifier)
	if name, ok := inputRef(tmpl); ok {
		return anyToString(runProps[name])
	}
	if !strings.Contains(tmpl, "{{") {
		return tmpl // a static identifier
	}
	return "" // a template we cannot evaluate (jq, concatenation, …)
}

// unresolvedRequired names blueprint-required properties whose mapped value
// came out empty. A hidden input defaulted from a jqQuery such as .user.email
// is the usual cause: it resolves to nothing when the action runs on client
// credentials rather than as a real user, and the upsert is then rejected.
func unresolvedRequired(c *client.Client, action *client.ActionDetail, runProps map[string]any) []string {
	bp, err := c.GetBlueprint(action.InvocationMethod.BlueprintIdentifier)
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
		input, ok := inputRef(anyToString(tmpl))
		if !ok {
			continue // a literal or expression we cannot evaluate; assume it resolved
		}
		if anyToString(runProps[input]) == "" {
			unresolved = append(unresolved,
				fmt.Sprintf("%s — from input %q, which resolved empty (pass --input %s=...)", name, input, input))
		}
	}
	sort.Strings(unresolved)
	return unresolved
}

// emitRunJSON prints a single, stable JSON object describing the run — enough to
// script against (run id, status, the entity identifier we set, linked entities)
// without scraping stderr or parsing multiple stdout blobs.
func emitRunJSON(result *client.ActionRun) {
	id, _ := result.Run.Properties["identifier"].(string)
	out, _ := json.MarshalIndent(map[string]any{
		"run_id":          result.Run.ID,
		"status":          result.Run.Status,
		"identifier":      id,
		"blueprint":       result.Run.Blueprint.Identifier,
		"linked_entities": result.GetLinkedEntities(),
	}, "", "  ")
	fmt.Println(string(out))
}

func printLinkedEntities(c *client.Client, result *client.ActionRun) {
	links := result.GetLinkedEntities()
	if len(links) == 0 {
		return
	}
	for _, link := range links {
		fmt.Fprintf(os.Stderr, "\nLinked entity: %s/%s\n", link.Blueprint, link.Identifier)
		entity, err := c.GetEntity(link.Blueprint, link.Identifier)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  (could not fetch: %v)\n", err)
			continue
		}
		out, _ := json.MarshalIndent(entity.Entity, "", "  ")
		fmt.Println(string(out))
	}
}

func parseInputs(inputs []string) (map[string]any, error) {
	props := make(map[string]any)
	for _, input := range inputs {
		parts := strings.SplitN(input, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid input format %q, expected key=value", input)
		}
		key, val := parts[0], parts[1]

		var jsonVal any
		if err := json.Unmarshal([]byte(val), &jsonVal); err == nil {
			props[key] = jsonVal
		} else {
			props[key] = val
		}
	}
	return props, nil
}
