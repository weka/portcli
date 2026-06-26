package cmd

import "github.com/spf13/cobra"

var entityCmd = &cobra.Command{
	Use:   "entity",
	Short: "Manage catalog entities",
}

func init() {
	entityCmd.AddCommand(entityGetCmd)
	entityCmd.AddCommand(entityUpdateCmd)
	entityCmd.AddCommand(entityDeleteCmd)
	entityCmd.AddCommand(entityWaitCmd)
}
