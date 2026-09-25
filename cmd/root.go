package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

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
	// Every client call takes a context, and this is where it comes from.
	// Tying it to the interrupt signals makes ^C abort an in-flight request
	// instead of killing the process partway through, say, a bulk PATCH.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		// A cancelled context means the user interrupted us; reporting it as a
		// failure would be noise on top of their own ^C.
		if errors.Is(err, context.Canceled) {
			os.Exit(130) // 128 + SIGINT, which is what a shell expects
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
