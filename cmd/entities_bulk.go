package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/weka/portcli/internal/bulk"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// resolveEntities lists the entities a --all/--filter operation applies to, and
// returns a description of the filter for the messages that mention it. An
// empty result is not an error: the caller decides what to say about it.
func resolveEntities(ctx context.Context, c *client.Client, blueprint, filter string) (entities []client.EntitySummary, filterDesc string, err error) {
	if filter == "" {
		entities, err = c.SearchEntities(ctx, blueprint)
	} else {
		field, value, ferr := portfmt.ParseFilter(filter)
		if ferr != nil {
			return nil, "", ferr
		}
		entities, err = c.SearchEntitiesWithFilter(ctx, blueprint, field, value)
		filterDesc = fmt.Sprintf(" matching filter %q", filter)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to list entities: %w", err)
	}
	return entities, filterDesc, nil
}

// entityIDs projects the identifiers a bulk operation works over.
func entityIDs(entities []client.EntitySummary) []string {
	ids := make([]string, len(entities))
	for i, e := range entities {
		ids[i] = e.Identifier
	}
	return ids
}

// runBulk applies fn across ids with the progress reporting shared by every
// bulk entity command: a line per entity as it lands, and a summary that fails
// the command if any did.
//
// verbPast is the capitalised past tense used per line ("Updated"); nounPlural
// names the operation in the failure summary ("updates").
func runBulk(ctx context.Context, ids []string, verbPast, nounPlural string, fn func(context.Context, string) error) error {
	failed := bulk.Apply(ctx, ids, bulk.DefaultConcurrency, fn,
		func(id string, err error) {
			if err != nil {
				fmt.Printf("  FAILED %s: %v\n", id, err)
				return
			}
			fmt.Printf("  %s %s\n", verbPast, id)
		})

	if failed > 0 {
		return fmt.Errorf("%d of %d %s failed", failed, len(ids), nounPlural)
	}
	fmt.Printf("All %d entities %s\n", len(ids), strings.ToLower(verbPast))
	return nil
}
