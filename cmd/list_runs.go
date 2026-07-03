package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/config"
)

var (
	listRunsEntity    string
	listRunsBlueprint string
	listRunsAction    string
	listRunsStatus    string
	listRunsLimit     int
	listRunsJSON      bool
)

var actionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List action runs",
	Long: `List Port action runs, optionally filtered by entity, blueprint, action identifier, or status.

Examples:
  portcli action list
  portcli action list --entity reg2463985 --blueprint selfservice-deployment
  portcli action list --entity reg2463985 --blueprint selfservice-deployment --action destroy
  portcli action list --entity reg2463985 --blueprint selfservice-deployment --status FAILURE --limit 10
  portcli action list --entity reg2463985 --blueprint selfservice-deployment --action destroy --json`,
	RunE: listRuns,
}

func init() {
	actionListCmd.Flags().StringVarP(&listRunsEntity, "entity", "e", "", "Filter by entity identifier")
	actionListCmd.Flags().StringVarP(&listRunsBlueprint, "blueprint", "b", "", "Blueprint identifier of the entity")
	actionListCmd.Flags().StringVarP(&listRunsAction, "action", "a", "", "Filter by action identifier (client-side)")
	actionListCmd.Flags().StringVarP(&listRunsStatus, "status", "s", "", "Filter by run status: IN_PROGRESS, SUCCESS, FAILURE (client-side, case-insensitive)")
	actionListCmd.Flags().IntVarP(&listRunsLimit, "limit", "l", 20, "Max number of runs to fetch from the API")
	actionListCmd.Flags().BoolVar(&listRunsJSON, "json", false, "Emit machine-readable JSON array of runs")
}

func listRuns(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	c := client.New(cfg)

	runs, err := c.ListActionRuns(listRunsEntity, listRunsBlueprint, listRunsLimit)
	if err != nil {
		return fmt.Errorf("failed to list runs: %w", err)
	}

	// Client-side filters.
	var filtered []client.RunSummary
	for _, r := range runs {
		if listRunsAction != "" && !strings.EqualFold(r.Action.Identifier, listRunsAction) {
			continue
		}
		if listRunsStatus != "" && !strings.EqualFold(r.Status, listRunsStatus) {
			continue
		}
		filtered = append(filtered, r)
	}

	// Sort newest-first by createdAt (ISO timestamps sort lexicographically).
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt > filtered[j].CreatedAt
	})

	if listRunsJSON {
		out, _ := json.MarshalIndent(filtered, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RUN_ID\tACTION\tSTATUS\tCREATED_AT\tENDED_AT")
	for _, r := range filtered {
		endedAt := ""
		if r.EndedAt != nil {
			endedAt = formatDateTime(*r.EndedAt)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			r.ID,
			r.Action.Identifier,
			r.Status,
			formatDateTime(r.CreatedAt),
			endedAt,
		)
	}
	w.Flush()
	return nil
}

// formatDateTime trims an ISO timestamp to date + time (no sub-seconds).
func formatDateTime(s string) string {
	if idx := strings.Index(s, "."); idx > 0 {
		s = s[:idx]
	}
	return strings.Replace(s, "T", " ", 1)
}
