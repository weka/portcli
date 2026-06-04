package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/spf13/cobra"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
)

var (
	updateAll    bool
	updateJSON   string
	updateFilter string
)

var entityUpdateCmd = &cobra.Command{
	Use:   "update <blueprint> [entity-identifier] [key=value ...]",
	Short: "Update properties on an entity or all entities of a blueprint",
	Long: `Update properties on a Port catalog entity.

Properties can be specified as key=value positional arguments, via --json, or both.
When both are used, --json values take precedence on conflicts.

Use --all to update the properties on every entity of the blueprint.
Use --filter to update only matching entities (implies --all).

Examples:
  portcli entity update myBlueprint my-entity status=active
  portcli entity update myBlueprint my-entity ttl="2026-06-01T09:13:26" status=active
  portcli entity update myBlueprint my-entity --json '{"status": "active", "metadata": {"nested": true}}'
  portcli entity update myBlueprint --all status=active
  portcli entity update myBlueprint --filter status=Failed ttl="2026-06-01T09:13:26"`,
	Args: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		filter, _ := cmd.Flags().GetString("filter")
		if all || filter != "" {
			if len(args) < 1 {
				return fmt.Errorf("requires at least 1 arg (blueprint) when using --all or --filter")
			}
		} else {
			if len(args) < 2 {
				return fmt.Errorf("requires at least 2 args (blueprint and entity-identifier)")
			}
		}
		return nil
	},
	RunE: updateEntity,
}

func init() {
	entityUpdateCmd.Flags().BoolVar(&updateAll, "all", false, "Update all entities of the blueprint")
	entityUpdateCmd.Flags().StringVar(&updateJSON, "json", "", "JSON object of properties to update")
	entityUpdateCmd.Flags().StringVarP(&updateFilter, "filter", "f", "", "Filter entities by property value (format: field=value), implies --all")
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

	bulkMode := updateAll || updateFilter != ""

	// Determine where key=value args start
	kvStart := 2
	if bulkMode {
		kvStart = 1
	}

	// Parse key=value pairs from remaining args
	props := make(map[string]any)
	for _, arg := range args[kvStart:] {
		idx := strings.IndexByte(arg, '=')
		if idx <= 0 {
			return fmt.Errorf("invalid argument %q: expected key=value format", arg)
		}
		key := arg[:idx]
		val := arg[idx+1:]
		props[key] = parseValue(val)
	}

	// Parse --json flag and overlay on top (takes precedence)
	if updateJSON != "" {
		var jsonProps map[string]any
		if err := json.Unmarshal([]byte(updateJSON), &jsonProps); err != nil {
			return fmt.Errorf("invalid --json value: %w", err)
		}
		for k, v := range jsonProps {
			props[k] = v
		}
	}

	if len(props) == 0 {
		return fmt.Errorf("no properties provided; use key=value arguments or --json flag")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	if bulkMode {
		return updateAllEntities(c, blueprint, props, updateFilter)
	}

	identifier := args[1]
	if err := c.UpdateEntityProperties(blueprint, identifier, props); err != nil {
		return fmt.Errorf("failed to update entity %s: %w", identifier, err)
	}
	fmt.Printf("Updated %s on entity %s\n", propNames(props), identifier)
	return nil
}

func propNames(props map[string]any) string {
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func updateAllEntities(c *client.Client, blueprint string, props map[string]any, filter string) error {
	var entities []client.EntitySummary
	var err error

	if filter != "" {
		field, value, ferr := parseFilter(filter)
		if ferr != nil {
			return ferr
		}
		entities, err = c.SearchEntitiesWithFilter(blueprint, field, value)
	} else {
		entities, err = c.SearchEntities(blueprint)
	}
	if err != nil {
		return fmt.Errorf("failed to list entities: %w", err)
	}

	if len(entities) == 0 {
		fmt.Println("No entities found")
		return nil
	}

	filterDesc := ""
	if filter != "" {
		filterDesc = fmt.Sprintf(" matching filter %q", filter)
	}
	fmt.Printf("Updating %s on %d entities%s...\n", propNames(props), len(entities), filterDesc)

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

			if err := c.UpdateEntityProperties(blueprint, id, props); err != nil {
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
