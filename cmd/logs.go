package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
)

var actionLogsCmd = &cobra.Command{
	Use:   "logs <run-id>",
	Short: "Get logs of an action run",
	Long: `Fetch logs for a Port action run.

Examples:
  portcli action logs r_MrfSMrQSXGEixIKO`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		c := client.New(cfg)
		logs, err := c.GetRunLogs(args[0])
		if err != nil {
			return fmt.Errorf("failed to get logs: %w", err)
		}

		// The endpoint answers 200 with an empty list for a run that does not
		// exist, so silence here is ambiguous — say which it is rather than
		// printing nothing at all.
		if len(logs) == 0 {
			fmt.Fprintf(os.Stderr, "no logs for run %s — it may have no log output, or may not exist "+
				"(Port keeps no run record for UPSERT_ENTITY actions)\n", args[0])
			return nil
		}

		for _, l := range logs {
			fmt.Printf("[%s] %s\n", l.CreatedAt, l.Message)
		}
		return nil
	},
}
