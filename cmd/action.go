package cmd

import "github.com/spf13/cobra"

var actionCmd = &cobra.Command{
	Use:   "action",
	Short: "Run and monitor self-service actions",
}

func init() {
	actionCmd.AddCommand(actionRunCmd)
	actionCmd.AddCommand(actionStatusCmd)
	actionCmd.AddCommand(actionLogsCmd)
	actionCmd.AddCommand(actionGetCmd)
	actionCmd.AddCommand(actionListCmd)
}
