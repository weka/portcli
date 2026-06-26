package cmd

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var (
	waitProperty string
	waitFor      string
	waitFailFor  string
	waitTimeout  int
	waitInterval int
)

var entityWaitCmd = &cobra.Command{
	Use:   "wait <blueprint> <entity-identifier>",
	Short: "Wait until an entity property matches a value",
	Long: `Poll a catalog entity until one of its properties matches a regex, or time out.

Matching is case-insensitive. Exits 0 when --for matches, 1 when --fail-for matches or
the timeout elapses. An entity that does not exist yet is treated as not-ready and polled
until it appears, so this can be started immediately after triggering an async action.

Examples:
  portcli entity wait selfservice-deployment-test my-run --for '^Passed$' --fail-for '^Failed$'
  portcli entity wait selfservice-deployment my-dep --for 'Running|Ready' --timeout 1200 --interval 15`,
	Args: cobra.ExactArgs(2),
	RunE: waitEntity,
}

func init() {
	entityWaitCmd.Flags().StringVarP(&waitProperty, "property", "p", "status", "Entity property to match against")
	entityWaitCmd.Flags().StringVar(&waitFor, "for", "", "Success regex — exit 0 when the property matches (required)")
	entityWaitCmd.Flags().StringVar(&waitFailFor, "fail-for", "", "Failure regex — exit 1 when the property matches")
	entityWaitCmd.Flags().IntVar(&waitTimeout, "timeout", 3600, "Timeout in seconds")
	entityWaitCmd.Flags().IntVar(&waitInterval, "interval", 20, "Poll interval in seconds")
}

func waitEntity(cmd *cobra.Command, args []string) error {
	blueprint, identifier := args[0], args[1]
	if waitFor == "" {
		return fmt.Errorf("--for is required")
	}
	// Match case-insensitively so callers don't have to know whether a backend reports
	// e.g. "Passed" vs "passed".
	successRE, err := regexp.Compile("(?i)" + waitFor)
	if err != nil {
		return fmt.Errorf("invalid --for regex: %w", err)
	}
	var failRE *regexp.Regexp
	if waitFailFor != "" {
		if failRE, err = regexp.Compile("(?i)" + waitFailFor); err != nil {
			return fmt.Errorf("invalid --fail-for regex: %w", err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	deadline := time.Now().Add(time.Duration(waitTimeout) * time.Second)
	fmt.Fprintf(os.Stderr, ">> waiting for %s/%s %s to match /%s/ (timeout %ds, interval %ds)\n",
		blueprint, identifier, waitProperty, waitFor, waitTimeout, waitInterval)

	for {
		val := ""
		entity, err := c.GetEntity(blueprint, identifier)
		if err != nil {
			// Not found yet (or transient) — keep polling; the timeout bounds the wait.
			fmt.Fprintf(os.Stderr, "   %s/%s not available yet (%v)\n", blueprint, identifier, err)
		} else {
			val = propStr(entity.Entity.Properties, waitProperty)
			fmt.Fprintf(os.Stderr, "   %s/%s %s=[%s]\n", blueprint, identifier, waitProperty, val)
			if failRE != nil && failRE.MatchString(val) {
				return fmt.Errorf("%s/%s %s=%q matched fail condition /%s/", blueprint, identifier, waitProperty, val, waitFailFor)
			}
			if successRE.MatchString(val) {
				fmt.Fprintf(os.Stderr, "OK: %s/%s %s=%q\n", blueprint, identifier, waitProperty, val)
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %ds waiting for %s/%s %s to match /%s/ (last=%q)",
				waitTimeout, blueprint, identifier, waitProperty, waitFor, val)
		}
		time.Sleep(time.Duration(waitInterval) * time.Second)
	}
}
