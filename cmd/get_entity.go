package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var getProperty string
var getFilter string

// parseFilter splits a "field=value" filter string into its parts.
func parseFilter(filter string) (field, value string, err error) {
	parts := strings.SplitN(filter, "=", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid filter format, expected field=value")
	}
	return parts[0], parts[1], nil
}

var entityGetCmd = &cobra.Command{
	Use:   "get <blueprint> [entity-identifier]",
	Short: "Get a catalog entity or list all entities",
	Long: `Fetch a Port catalog entity by blueprint and identifier.
If no entity identifier is provided, lists all entities for the blueprint.

Examples:
  portcli entity get myBlueprint
  portcli entity get myBlueprint my-entity-id
  portcli entity get myBlueprint my-entity-id --property version
  portcli entity get myBlueprint my-entity-id -p status
  portcli entity get myBlueprint --filter "environment=production"`,
	Args: cobra.RangeArgs(1, 2),
	RunE: getEntity,
}

func init() {
	entityGetCmd.Flags().StringVarP(&getProperty, "property", "p", "", "Print only the value of a specific property")
	entityGetCmd.Flags().StringVarP(&getFilter, "filter", "f", "", "Filter entities by property value (format: field=value)")
}

func getEntity(cmd *cobra.Command, args []string) error {
	blueprint := args[0]

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	if getFilter != "" && len(args) == 2 {
		return fmt.Errorf("--filter cannot be used with a specific entity identifier")
	}

	if len(args) == 1 {
		if getFilter != "" {
			field, value, err := parseFilter(getFilter)
			if err != nil {
				return err
			}
			entities, err := c.SearchEntitiesWithFilter(blueprint, field, value)
			if err != nil {
				return fmt.Errorf("failed to search entities: %w", err)
			}
			out, _ := json.MarshalIndent(entities, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		entities, err := c.SearchEntities(blueprint)
		if err != nil {
			return fmt.Errorf("failed to list entities: %w", err)
		}
		out, _ := json.MarshalIndent(entities, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	identifier := args[1]

	entity, err := c.GetEntity(blueprint, identifier)
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
