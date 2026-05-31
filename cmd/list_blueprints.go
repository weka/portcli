package cmd

import (
	"fmt"
	"regexp"

	"github.com/weka/portcli/internal/config"
	"github.com/weka/portcli/internal/client"
	"github.com/spf13/cobra"
)

var filterRegex string

var blueprintListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all blueprints",
	Long: `List all Port blueprints, optionally filtered by a regex pattern.

The regex is matched against the blueprint identifier.

Examples:
  portcli blueprint list
  portcli blueprint list --filter "^service"
  portcli blueprint list --filter "deploy|build"`,
	RunE: listBlueprints,
}

func init() {
	blueprintListCmd.Flags().StringVar(&filterRegex, "filter", "", "Regex pattern to filter blueprint identifiers")
}

func listBlueprints(cmd *cobra.Command, args []string) error {
	var re *regexp.Regexp
	if filterRegex != "" {
		var err error
		re, err = regexp.Compile(filterRegex)
		if err != nil {
			return fmt.Errorf("invalid regex %q: %w", filterRegex, err)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	blueprints, err := c.ListBlueprints()
	if err != nil {
		return fmt.Errorf("failed to list blueprints: %w", err)
	}

	for _, bp := range blueprints {
		if re != nil && !re.MatchString(bp.Identifier) {
			continue
		}
		fmt.Printf("%-30s  %s\n", bp.Identifier, bp.Title)
	}
	return nil
}
