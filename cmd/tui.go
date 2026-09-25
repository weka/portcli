package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/weka/portcli/internal/tui"
)

var (
	tuiRefresh time.Duration
	tuiView    string
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive terminal UI",
	Long: `Open the interactive terminal UI for browsing blueprints, entities and action
runs, and for running actions.

This is also what a bare "portcli" does when stdin and stdout are a terminal.
Press ":" for the command palette, "/" to filter, and "?" for the key map.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !isInteractive() {
			// An explicit request we cannot honour is an error, unlike the
			// bare-invocation fallback, which is a guess worth correcting
			// quietly.
			return fmt.Errorf("portcli tui needs stdin and stdout to be a terminal")
		}
		return startTUI(cmd)
	},
}

func init() {
	tuiCmd.Flags().DurationVar(&tuiRefresh, "refresh", 0, "Poll interval, overriding the per-resource defaults (e.g. 10s)")
	tuiCmd.Flags().StringVar(&tuiView, "view", "", "Open this view instead of the remembered one (e.g. \"entities my-blueprint\")")
	rootCmd.AddCommand(tuiCmd)
}

// isInteractive reports whether there is a terminal to drive a TUI with. Both
// ends matter: tcell reads keys from stdin and paints to stdout, so a redirect
// on either one leaves it with nothing to do.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// startTUI loads credentials once and hands the TUI a single client, so every
// view shares one token — and one re-authentication when it expires.
func startTUI(cmd *cobra.Command) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	// Fail like a CLI before taking over the screen, so bad credentials
	// produce the same message here as everywhere else rather than a
	// full-screen app that cannot load anything. A short budget keeps a slow
	// network from looking like a hang.
	probeCtx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	_, probeErr := c.ListBlueprints(probeCtx)
	cancel()
	if probeErr != nil && client.IsUnauthorized(probeErr) {
		return fmt.Errorf("authentication failed for %s: %w", cfg.BaseURL, probeErr)
	}
	// A timeout or DNS failure still enters the TUI: the header reports it and
	// ctrl-r retries, which beats refusing to start over a transient blip.

	return tui.Run(cmd.Context(), c, tui.Options{
		Version: Version,
		Config:  cfg,
		Refresh: tuiRefresh,
		View:    tuiView,
	})
}
