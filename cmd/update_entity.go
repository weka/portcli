package cmd

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var (
	updateAll  bool
	fieldName  string
	fieldValue string
)

var entityUpdateCmd = &cobra.Command{
	Use:   "update <blueprint> [entity-identifier]",
	Short: "Update a field on an entity or all entities of a blueprint",
	Long: `Update a single property on a Port catalog entity.

Use --all to update the field on every entity of the blueprint.

Examples:
  portcli entity update myBlueprint my-entity --field status --value active
  portcli entity update myBlueprint --all --field status --value active`,
	Args: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all {
			return cobra.ExactArgs(1)(cmd, args)
		}
		return cobra.ExactArgs(2)(cmd, args)
	},
	RunE: updateEntity,
}

func init() {
	entityUpdateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all entities of the blueprint")
	entityUpdateCmd.Flags().StringVar(&fieldName, "field", "", "Property name to update")
	entityUpdateCmd.Flags().StringVar(&fieldValue, "value", "", "New value (JSON-parsed if possible, otherwise string)")
	entityUpdateCmd.MarkFlagRequired("field")
	entityUpdateCmd.MarkFlagRequired("value")
}

func parseValue(raw string) any {
	var parsed any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return raw
}

func updateEntity(cmd *cobra.Command, args []string) error {
	blueprint := args[0]
	value := parseValue(fieldValue)

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	if updateAll {
		return updateAllEntities(c, blueprint, fieldName, value)
	}

	identifier := args[1]
	if err := c.UpdateEntityProperty(blueprint, identifier, fieldName, value); err != nil {
		return fmt.Errorf("failed to update entity %s: %w", identifier, err)
	}
	fmt.Printf("Updated %s.%s on entity %s\n", blueprint, fieldName, identifier)
	return nil
}

func updateAllEntities(c *client.Client, blueprint, field string, value any) error {
	entities, err := c.SearchEntities(blueprint)
	if err != nil {
		return fmt.Errorf("failed to list entities: %w", err)
	}

	if len(entities) == 0 {
		fmt.Println("No entities found")
		return nil
	}

	fmt.Printf("Updating %s on %d entities...\n", field, len(entities))

	var (
		failed atomic.Int32
		mu     sync.Mutex
		sem    = make(chan struct{}, 5)
		wg     sync.WaitGroup
	)

	for _, entity := range entities {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := c.UpdateEntityProperty(blueprint, id, field, value); err != nil {
				mu.Lock()
				fmt.Printf("  FAILED %s: %v\n", id, err)
				mu.Unlock()
				failed.Add(1)
				return
			}
			mu.Lock()
			fmt.Printf("  Updated %s\n", id)
			mu.Unlock()
		}(entity.Identifier)
	}

	wg.Wait()

	f := int(failed.Load())
	if f > 0 {
		return fmt.Errorf("%d of %d updates failed", f, len(entities))
	}
	fmt.Printf("All %d entities updated\n", len(entities))
	return nil
}
