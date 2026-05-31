package cmd

import "github.com/spf13/cobra"

var blueprintCmd = &cobra.Command{
	Use:   "blueprint",
	Short: "Manage blueprints",
}

func init() {
	blueprintCmd.AddCommand(blueprintListCmd)
}
