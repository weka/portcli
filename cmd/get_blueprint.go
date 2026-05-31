package cmd

import (
	"fmt"
	"sort"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var blueprintGetCmd = &cobra.Command{
	Use:   "get <identifier>",
	Short: "Get blueprint field names",
	Long: `Fetch a Port blueprint and display its schema properties in a table.

Examples:
  portcli blueprint get myBlueprint`,
	Args: cobra.ExactArgs(1),
	RunE: getBlueprint,
}

func init() {
	blueprintCmd.AddCommand(blueprintGetCmd)
}

func getBlueprint(cmd *cobra.Command, args []string) error {
	identifier := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)
	bp, err := c.GetBlueprint(identifier)
	if err != nil {
		return fmt.Errorf("failed to get blueprint: %w", err)
	}

	keys := make([]string, 0, len(bp.Schema.Properties))
	for k := range bp.Schema.Properties {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf("%-30s  %-30s  %s\n", "FIELD", "TITLE", "TYPE")
	for _, k := range keys {
		p := bp.Schema.Properties[k]
		fmt.Printf("%-30s  %-30s  %s\n", k, p.Title, p.Type)
	}
	return nil
}
