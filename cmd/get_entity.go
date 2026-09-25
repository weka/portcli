package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/weka/portcli/internal/portfmt"
)

var getProperty string
var getFilter string
var getColumns string

var entityGetCmd = &cobra.Command{
	Use:   "get <blueprint> [entity-identifier]",
	Short: "Get a catalog entity or list all entities",
	Long: `Fetch a Port catalog entity by blueprint and identifier.
If no entity identifier is provided, lists all entities for the blueprint.

Examples:
  portcli entity get myBlueprint
  portcli entity get myBlueprint --columns status,owner,deploymentType
  portcli entity get myBlueprint my-entity-id
  portcli entity get myBlueprint my-entity-id --property version
  portcli entity get myBlueprint my-entity-id -p status
  portcli entity get myBlueprint --filter "environment=production" --columns status`,
	Args: cobra.RangeArgs(1, 2),
	RunE: getEntity,
}

func init() {
	entityGetCmd.Flags().StringVarP(&getProperty, "property", "p", "", "Print only the value of a specific property")
	entityGetCmd.Flags().StringVarP(&getFilter, "filter", "f", "", "Filter entities by property value (format: field=value)")
	entityGetCmd.Flags().StringVarP(&getColumns, "columns", "c", "", "Comma-separated list of property columns to show (e.g. status,owner,deploymentType)")
}

func getEntity(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	blueprint := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	if getFilter != "" && len(args) == 2 {
		return fmt.Errorf("--filter cannot be used with a specific entity identifier")
	}
	if getColumns != "" && len(args) == 2 {
		return fmt.Errorf("--columns cannot be used with a specific entity identifier")
	}

	if len(args) == 1 {
		var entities []client.EntitySummary
		if getFilter != "" {
			field, value, err := portfmt.ParseFilter(getFilter)
			if err != nil {
				return err
			}
			entities, err = c.SearchEntitiesWithFilter(ctx, blueprint, field, value)
			if err != nil {
				return fmt.Errorf("failed to search entities: %w", err)
			}
		} else {
			var err error
			entities, err = c.SearchEntities(ctx, blueprint)
			if err != nil {
				return fmt.Errorf("failed to list entities: %w", err)
			}
		}
		var columns []string
		if getColumns != "" {
			for _, col := range strings.Split(getColumns, ",") {
				col = strings.TrimSpace(col)
				if col != "" {
					columns = append(columns, col)
				}
			}
		}
		// Always include createdAt column
		hasCreatedAt := false
		for _, col := range columns {
			if strings.ToLower(col) == "createdat" {
				hasCreatedAt = true
				break
			}
		}
		if !hasCreatedAt {
			columns = append(columns, "createdAt")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		header := "IDENTIFIER"
		for _, col := range columns {
			header += "\t" + strings.ToUpper(portfmt.CamelToSnake(col))
		}
		fmt.Fprintln(w, header)
		for _, e := range entities {
			row := e.Identifier
			for _, col := range columns {
				row += "\t" + portfmt.EntityCol(e, col)
			}
			fmt.Fprintln(w, row)
		}
		w.Flush()
		return nil
	}

	identifier := args[1]

	entity, err := c.GetEntity(ctx, blueprint, identifier)
	if err != nil {
		return fmt.Errorf("failed to get entity: %w", err)
	}

	if getProperty != "" {
		val, ok := entity.Entity.Properties[getProperty]
		if !ok {
			return fmt.Errorf("property %q not found on entity", getProperty)
		}
		if s, ok := val.(string); ok {
			fmt.Println(s)
		} else {
			out, _ := json.Marshal(val)
			fmt.Println(string(out))
		}
		return nil
	}

	out, _ := json.MarshalIndent(entity.Entity, "", "  ")
	fmt.Println(string(out))
	return nil
}
