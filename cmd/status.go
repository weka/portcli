package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var actionStatusCmd = &cobra.Command{
	Use:   "status <run-id>",
	Short: "Get the status of an action run",
	Long: `Fetch the current status and details of a Port action run.

Examples:
  portcli action status r_MrfSMrQSXGEixIKO`,
	Args: cobra.ExactArgs(1),
	RunE: getStatus,
}

func getStatus(cmd *cobra.Command, args []string) error {
	runID := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	result, err := c.GetActionRun(runID)
	if err != nil {
		return fmt.Errorf("failed to get run: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Run %s: %s\n", runID, result.Run.Status)

	out, _ := json.MarshalIndent(result.Run, "", "  ")
	fmt.Println(string(out))

	printLinkedEntities(c, result)
	return nil
}
