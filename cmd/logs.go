package cmd

import (
	"fmt"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
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

		for _, l := range logs {
			fmt.Printf("[%s] %s\n", l.CreatedAt, l.Message)
		}
		return nil
	},
}
