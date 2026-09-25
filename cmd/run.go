package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/weka/portcli/internal/portfmt"
	"github.com/weka/portcli/internal/upsert"
)

var (
	inputFlags  []string
	wait        bool
	pollSeconds int
	timeoutSecs int
	runAs       string
	entityID    string
	identifier  string
	runJSON     bool
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

	properties, err := portfmt.ParseInputs(inputFlags)
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
	upserted, err := upsert.Verify(c, upsert.Request{
		ActionID:   actionID,
		RunID:      runID,
		Identifier: identifier,
		RunProps:   result.Run.Properties,
	})
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
	for _, resolved := range c.ResolveLinks(result) {
		fmt.Fprintf(os.Stderr, "\nLinked entity: %s/%s\n", resolved.Link.Blueprint, resolved.Link.Identifier)
		if resolved.Err != nil {
			fmt.Fprintf(os.Stderr, "  (could not fetch: %v)\n", resolved.Err)
			continue
		}
		out, _ := json.MarshalIndent(resolved.Entity.Entity, "", "  ")
		fmt.Println(string(out))
	}
}
