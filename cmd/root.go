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
	Use:   "portcli",
	Short: "CLI for Port.io self-service actions",
	Long: `portcli drives Port.io from the terminal.

With no arguments on a terminal it opens the interactive UI; with a subcommand
it behaves as a scriptable CLI. "portcli --help" always prints this text.`,
	Version: Version,
	// Execute prints the error itself; without SilenceErrors cobra prints it
	// too, doubling every failure message.
	SilenceUsage:  true,
	SilenceErrors: true,
	// Cobra applies this default inside its own unknown-command path, which
	// rootArgs now replaces — left at zero, SuggestionsFor matches nothing and
	// "Did you mean this?" silently disappears.
	SuggestionsMinimumDistance: 2,
	Args:                       rootArgs,
	RunE:                       runRoot,
}

// rootArgs restores the unknown-command check cobra applies to a root with no
// Run. Cobra installs that check inside Find() only while Args is nil, so
// giving root a RunE without reinstating it would turn every typo —
// "portcli entty" — into a silent TUI launch.
func rootArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
	if suggestions := cmd.SuggestionsFor(args[0]); len(suggestions) > 0 {
		msg += "\n\nDid you mean this?\n"
		for _, name := range suggestions {
			msg += "\t" + name + "\n"
		}
	}
	return errors.New(msg)
}

// runRoot is what a bare "portcli" does. On a terminal that is the TUI. With
// either end redirected there is nothing to draw on, so fall back to the help
// text a non-runnable root used to print — same stream, same exit status — so
// pipelines and CI smoke checks that run bare "portcli" keep working.
func runRoot(cmd *cobra.Command, _ []string) error {
	if !isInteractive() {
		return cmd.Help()
	}
	return startTUI(cmd)
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
