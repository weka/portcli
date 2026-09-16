package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags.
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "portcli",
	Short:   "CLI for Port.io self-service actions",
	Version: Version,
	// Execute prints the error itself; without SilenceErrors cobra prints it
	// too, doubling every failure message.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(blueprintCmd)
	rootCmd.AddCommand(entityCmd)
	rootCmd.AddCommand(actionCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
