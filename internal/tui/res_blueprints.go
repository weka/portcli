package tui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// blueprintsResource lists the catalog's blueprints. Adding a resource is one
// file shaped like this one: the type, its Columns/List/Ops, and an init that
// registers it.
type blueprintsResource struct{}

func init() {
	Register("blueprints", []string{"bp"}, func([]string) (Resource, error) {
		return blueprintsResource{}, nil
	})
}

func (blueprintsResource) Kind() string  { return "blueprints" }
func (blueprintsResource) ID() string    { return "blueprints" }
func (blueprintsResource) Title() string { return "blueprints" }

func (blueprintsResource) Columns() []Column {
	return []Column{
		{Name: "IDENTIFIER"},
		{Name: "TITLE"},
		{Name: "DESCRIPTION", Wide: true},
		{Name: "UPDATED"},
	}
}

func (blueprintsResource) List(ctx context.Context, c *client.Client) ([]Row, error) {
	bps, err := c.ListBlueprints(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(bps))
	for _, bp := range bps {
		rows = append(rows, Row{
			ID:  bp.Identifier,
			Obj: bp,
			Cells: []string{
				bp.Identifier,
				bp.Title,
				bp.Description,
				portfmt.DateTime(bp.UpdatedAt),
			},
		})
	}
	return rows, nil
}

func (blueprintsResource) Ops() []Op {
	return []Op{
		{
			Key:  tcell.KeyEnter,
			Name: "Entities",
			Run: func(a *App, rows []Row) error {
				return a.open("entities " + rows[0].ID)
			},
		},
		{
			Rune: 'd',
			Name: "Describe",
			Run: func(a *App, rows []Row) error {
				// The schema is what someone opening a blueprint wants, and
				// the list payload does not carry it.
				detail, err := a.client.GetBlueprint(a.ctx, rows[0].ID)
				if err != nil {
					return fmt.Errorf("cannot fetch blueprint %s: %w", rows[0].ID, err)
				}
				a.push(newDescribeView(a, rows[0].ID, rows[0].ID, detail))
				return nil
			},
		},
	}
}
