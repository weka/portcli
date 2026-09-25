package tui

import (
	"context"
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// runsLimit is how many runs to fetch. The endpoint pages, but a screenful of
// recent activity is what this view is for.
const runsLimit = 100

// runsResource lists action runs, optionally scoped to one entity.
type runsResource struct {
	blueprint string
	entity    string
}

func init() {
	Register("runs", []string{"run", "r"}, func(args []string) (Resource, error) {
		r := &runsResource{}
		// ":runs" for everything, ":runs <blueprint> <entity>" for one entity —
		// the endpoint needs both to scope by entity.
		switch len(args) {
		case 0:
		case 1:
			r.blueprint = args[0]
		default:
			r.blueprint, r.entity = args[0], args[1]
		}
		return r, nil
	})
}

func (r *runsResource) Kind() string { return "runs" }

func (r *runsResource) ID() string {
	switch {
	case r.entity != "":
		return fmt.Sprintf("runs %s %s", r.blueprint, r.entity)
	case r.blueprint != "":
		return "runs " + r.blueprint
	default:
		return "runs"
	}
}

func (r *runsResource) Title() string {
	if r.entity != "" {
		return fmt.Sprintf("runs(%s)", r.entity)
	}
	if r.blueprint != "" {
		return fmt.Sprintf("runs(%s)", r.blueprint)
	}
	return "runs"
}

func (r *runsResource) Columns() []Column {
	return []Column{
		{Name: "RUN_ID"},
		{Name: "ACTION"},
		{Name: "STATUS"},
		{Name: "CREATED_AT"},
		{Name: "ENDED_AT"},
	}
}

func (r *runsResource) List(ctx context.Context, c *client.Client) ([]Row, error) {
	runs, err := c.ListActionRuns(ctx, r.entity, r.blueprint, runsLimit)
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, len(runs))
	for _, run := range runs {
		ended := "—"
		if run.EndedAt != nil {
			ended = portfmt.DateTime(*run.EndedAt)
		}
		rows = append(rows, Row{
			ID:  run.ID,
			Obj: run,
			Cells: []string{
				run.ID,
				run.Action.Identifier,
				run.Status,
				portfmt.DateTime(run.CreatedAt),
				ended,
			},
		})
	}
	return rows, nil
}

func (r *runsResource) Ops() []Op {
	return []Op{
		{
			Key:  tcell.KeyEnter,
			Name: "Logs",
			Run:  func(a *App, rows []Row) error { return openLogs(a, rows) },
		},
		{
			Rune: 'l',
			Name: "Logs",
			Run:  func(a *App, rows []Row) error { return openLogs(a, rows) },
		},
		{
			Rune: 'd',
			Name: "Describe",
			Run: func(a *App, rows []Row) error {
				run, err := a.client.GetActionRun(a.ctx, rows[0].ID)
				if err != nil {
					// Port keeps no run record for UPSERT_ENTITY actions, so
					// these ids 404 by design; say so rather than reporting a
					// bare not-found.
					return upsertAwareError(err)
				}
				a.push(newDescribeView(a, "run/"+rows[0].ID, rows[0].ID, run.Run))
				return nil
			},
		},
	}
}

func openLogs(a *App, rows []Row) error {
	a.push(newLogsView(a, rows[0].ID))
	return nil
}
