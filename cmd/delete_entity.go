package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
	"github.com/spf13/cobra"
)

var (
	deleteAll     bool
	deleteConfirm bool
	deleteFilter  string
)

var entityDeleteCmd = &cobra.Command{
	Use:   "delete <blueprint> [entity-identifier]",
	Short: "Delete an entity",
	Long: `Delete a Port catalog entity.

Use --all to delete every entity of the blueprint.
Use --filter with --all to delete only matching entities.

Examples:
  portcli entity delete myBlueprint my-entity
  portcli entity delete myBlueprint my-entity --yes
  portcli entity delete myBlueprint --all
  portcli entity delete myBlueprint --all --yes
  portcli entity delete myBlueprint --all --filter "owner=user@example.com"`,
	Args: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all {
			return cobra.ExactArgs(1)(cmd, args)
		}
		return cobra.ExactArgs(2)(cmd, args)
	},
	RunE: deleteEntity,
}

func init() {
	entityDeleteCmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all entities of the blueprint")
	entityDeleteCmd.Flags().BoolVarP(&deleteConfirm, "yes", "y", false, "Skip confirmation prompt")
	entityDeleteCmd.Flags().StringVarP(&deleteFilter, "filter", "f", "", "Filter entities by property value (format: field=value), requires --all")
}

func confirmPrompt(message string) bool {
	fmt.Print(message)
	reader := bufio.NewReader(os.Stdin)
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "y" || answer == "yes"
}

func deleteEntity(cmd *cobra.Command, args []string) error {
	blueprint := args[0]

	if deleteFilter != "" && !deleteAll {
		return fmt.Errorf("--filter requires --all")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c := client.New(cfg)

	if deleteAll {
		return deleteAllEntities(c, blueprint)
	}

	identifier := args[1]

	if !deleteConfirm {
		if !confirmPrompt(fmt.Sprintf("Are you sure you want to delete entity %s from blueprint %s? (y/N): ", identifier, blueprint)) {
			fmt.Println("Aborted")
			return nil
		}
	}

	if err := c.DeleteEntity(blueprint, identifier); err != nil {
		return fmt.Errorf("failed to delete entity %s: %w", identifier, err)
	}
	fmt.Printf("Deleted entity %s from blueprint %s\n", identifier, blueprint)
	return nil
}

func deleteAllEntities(c *client.Client, blueprint string) error {
	var entities []client.EntitySummary
	var err error

	if deleteFilter != "" {
		field, value, ferr := parseFilter(deleteFilter)
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
	if deleteFilter != "" {
		filterDesc = fmt.Sprintf(" matching filter %q", deleteFilter)
	}

	if !deleteConfirm {
		if !confirmPrompt(fmt.Sprintf("Are you sure you want to delete %d entities%s from blueprint %s? (y/N): ", len(entities), filterDesc, blueprint)) {
			fmt.Println("Aborted")
			return nil
		}
	}

	fmt.Printf("Deleting %d entities...\n", len(entities))

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

			if err := c.DeleteEntity(blueprint, id); err != nil {
				mu.Lock()
				fmt.Printf("  FAILED %s: %v\n", id, err)
				mu.Unlock()
				failed.Add(1)
				return
			}
			mu.Lock()
			fmt.Printf("  Deleted %s\n", id)
			mu.Unlock()
		}(entity.Identifier)
	}

	wg.Wait()

	f := int(failed.Load())
	if f > 0 {
		return fmt.Errorf("%d of %d deletes failed", f, len(entities))
	}
	fmt.Printf("All %d entities deleted\n", len(entities))
	return nil
}
