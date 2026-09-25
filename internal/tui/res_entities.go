package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/weka/portcli/internal/bulk"
	"github.com/weka/portcli/internal/client"
	"github.com/weka/portcli/internal/portfmt"
)

// entitiesResource lists a blueprint's entities.
type entitiesResource struct {
	blueprint string
	// filter is a server-side field=value, pushed to the search endpoint
	// rather than applied to the rows we already have.
	filterField string
	filterValue string
	filterRaw   string
	// cols, when set, overrides the auto-chosen columns (":cols status,owner").
	cols []string
}

func init() {
	Register("entities", []string{"ent", "e"}, func(args []string) (Resource, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("which blueprint? try :entities <blueprint>")
		}
		return &entitiesResource{blueprint: args[0]}, nil
	})
}

func (r *entitiesResource) Kind() string { return "entities" }

func (r *entitiesResource) ID() string {
	id := "entities " + r.blueprint
	if r.filterRaw != "" {
		id += " " + r.filterRaw
	}
	return id
}

func (r *entitiesResource) Title() string {
	if r.filterRaw != "" {
		return fmt.Sprintf("entities(%s|%s)", r.blueprint, r.filterRaw)
	}
	return fmt.Sprintf("entities(%s)", r.blueprint)
}

// SetServerFilter records a field=value to push to the search endpoint.
//
// Server-side filtering is kept distinct from "/" on purpose: this hits a
// different endpoint, and the field name needs the "$" translation for an
// entity's top-level fields, which the client applies. Conflating the two
// would make one of them silently wrong.
func (r *entitiesResource) SetServerFilter(filter string) error {
	field, value, err := portfmt.ParseFilter(filter)
	if err != nil {
		return err
	}
	r.filterField, r.filterValue, r.filterRaw = field, value, filter
	return nil
}

// SetColumns overrides the auto-chosen property columns.
func (r *entitiesResource) SetColumns(cols []string) { r.cols = cols }

// Columns is the pre-fetch guess. The real set comes from ListWithColumns
// once the blueprint schema has been read.
func (r *entitiesResource) Columns() []Column {
	return []Column{{Name: "IDENTIFIER"}, {Name: "TITLE"}, {Name: "CREATED_AT"}}
}

func (r *entitiesResource) List(ctx context.Context, c *client.Client) ([]Row, error) {
	_, rows, err := r.ListWithColumns(ctx, c)
	return rows, err
}

// ListWithColumns fetches the entities and works out which properties are
// worth showing. The CLI makes --columns mandatory; here the blueprint schema
// is already available, so a useful default costs one extra request.
func (r *entitiesResource) ListWithColumns(ctx context.Context, c *client.Client) ([]Column, []Row, error) {
	var (
		entities []client.EntitySummary
		err      error
	)
	if r.filterRaw != "" {
		entities, err = c.SearchEntitiesWithFilter(ctx, r.blueprint, r.filterField, r.filterValue)
	} else {
		entities, err = c.SearchEntities(ctx, r.blueprint)
	}
	if err != nil {
		return nil, nil, err
	}

	props := r.cols
	if props == nil {
		// A failed schema read is not fatal: fall back to the properties the
		// entities themselves carry.
		bp, bpErr := c.GetBlueprint(ctx, r.blueprint)
		if bpErr == nil {
			props = scalarColumns(bp)
		} else {
			props = propertiesSeen(entities)
		}
	}

	cols := []Column{{Name: "IDENTIFIER"}, {Name: "TITLE"}}
	for _, p := range props {
		cols = append(cols, Column{Name: strings.ToUpper(portfmt.CamelToSnake(p))})
	}
	cols = append(cols,
		Column{Name: "CREATED_AT"},
		Column{Name: "CREATED_BY", Wide: true},
	)

	rows := make([]Row, 0, len(entities))
	for _, e := range entities {
		cells := []string{e.Identifier, e.Title}
		for _, p := range props {
			cells = append(cells, portfmt.EntityCol(e, p))
		}
		cells = append(cells, portfmt.Date(e.CreatedAt), e.CreatedBy)
		rows = append(rows, Row{ID: e.Identifier, Obj: e, Cells: cells})
	}
	return cols, rows, nil
}

// preferredColumns are property names worth showing before anything else,
// because they are what someone scanning a list of entities is looking for.
var preferredColumns = []string{"status", "state", "health", "owner", "environment", "cloud", "version", "ttl"}

// maxAutoColumns keeps the table readable on a normal terminal.
const maxAutoColumns = 4

// scalarColumns picks property columns from a blueprint schema. Only scalars:
// an object or array renders as a wall of JSON that crowds out everything
// else.
func scalarColumns(bp *client.BlueprintDetail) []string {
	scalar := map[string]bool{}
	for name, p := range bp.Schema.Properties {
		switch p.Type {
		case "string", "number", "integer", "boolean":
			scalar[name] = true
		}
	}

	var chosen []string
	for _, want := range preferredColumns {
		for name := range scalar {
			if strings.EqualFold(name, want) {
				chosen = append(chosen, name)
				delete(scalar, name)
				break
			}
		}
	}

	rest := make([]string, 0, len(scalar))
	for name := range scalar {
		rest = append(rest, name)
	}
	sort.Strings(rest)
	chosen = append(chosen, rest...)

	if len(chosen) > maxAutoColumns {
		chosen = chosen[:maxAutoColumns]
	}
	return chosen
}

// propertiesSeen is the fallback when the schema cannot be read: use whatever
// keys the entities actually carry.
func propertiesSeen(entities []client.EntitySummary) []string {
	seen := map[string]bool{}
	for _, e := range entities {
		for k := range e.Properties {
			seen[k] = true
		}
	}
	names := make([]string, 0, len(seen))
	for k := range seen {
		names = append(names, k)
	}
	sort.Strings(names)

	var chosen []string
	for _, want := range preferredColumns {
		for _, name := range names {
			if strings.EqualFold(name, want) {
				chosen = append(chosen, name)
			}
		}
	}
	for _, name := range names {
		if len(chosen) >= maxAutoColumns {
			break
		}
		if !contains(chosen, name) {
			chosen = append(chosen, name)
		}
	}
	if len(chosen) > maxAutoColumns {
		chosen = chosen[:maxAutoColumns]
	}
	return chosen
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func (r *entitiesResource) Ops() []Op {
	return []Op{
		{
			Key:  tcell.KeyEnter,
			Name: "Describe",
			Run:  func(a *App, rows []Row) error { return r.describe(a, rows) },
		},
		{
			Rune: 'd',
			Name: "Describe",
			Run:  func(a *App, rows []Row) error { return r.describe(a, rows) },
		},
		{
			Rune: 'l',
			Name: "Runs",
			Run: func(a *App, rows []Row) error {
				return a.open(fmt.Sprintf("runs %s %s", r.blueprint, rows[0].ID))
			},
		},
		{
			Rune: 'e',
			Name: "Edit in $EDITOR",
			Run:  func(a *App, rows []Row) error { return r.edit(a, rows) },
		},
		{
			Key:       tcell.KeyCtrlD,
			Name:      "Delete",
			Dangerous: true,
			Run:       func(a *App, rows []Row) error { return r.delete(a, rows) },
		},
	}
}

// edit opens the entity's properties in the user's editor and PATCHes only
// what changed.
func (r *entitiesResource) edit(a *App, rows []Row) error {
	if len(rows) != 1 {
		// Editing several entities at once would mean applying one person's
		// edits to documents they never saw.
		return fmt.Errorf("edit works on one entity at a time; %d are marked", len(rows))
	}
	id := rows[0].ID

	entity, err := a.client.GetEntity(a.ctx, r.blueprint, id)
	if err != nil {
		return fmt.Errorf("cannot fetch entity %s: %w", id, err)
	}
	before := entity.Entity.Properties
	if before == nil {
		before = map[string]any{}
	}

	after, saved, err := a.editJSONInEditor(id, before)
	if err != nil {
		return err
	}
	if !saved {
		a.flash.show(flashInfo, "edit cancelled")
		return nil
	}

	changed := Diff(before, after)
	if len(changed) == 0 {
		a.flash.show(flashInfo, "no changes")
		return nil
	}

	if err := a.client.UpdateEntityProperties(a.ctx, r.blueprint, id, changed); err != nil {
		return fmt.Errorf("cannot update %s: %w", id, err)
	}
	a.flash.show(flashInfo, "updated %s on %s", DiffSummary(changed), id)
	if t, ok := a.table(); ok {
		t.refreshNow(a.ctx)
	}
	return nil
}

// delete removes the selected entities. Reached only through the confirmation
// modal, because Op.Dangerous is set.
func (r *entitiesResource) delete(a *App, rows []Row) error {
	ids := make([]string, len(rows))
	for i, row := range rows {
		ids[i] = row.ID
	}

	if len(ids) == 1 {
		if err := a.client.DeleteEntity(a.ctx, r.blueprint, ids[0]); err != nil {
			return fmt.Errorf("cannot delete %s: %w", ids[0], err)
		}
		a.flash.show(flashInfo, "deleted %s", ids[0])
		r.afterDelete(a)
		return nil
	}

	// Same bounded concurrency as the CLI's --all, so a bulk delete from here
	// puts no more load on Port than one from a script.
	var failures []string
	failed := bulk.Apply(a.ctx, ids, bulk.DefaultConcurrency,
		func(ctx context.Context, id string) error {
			return a.client.DeleteEntity(ctx, r.blueprint, id)
		},
		func(id string, err error) {
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", id, err))
			}
		})

	r.afterDelete(a)
	if failed > 0 {
		return fmt.Errorf("%d of %d deletes failed:\n%s", failed, len(ids), strings.Join(failures, "\n"))
	}
	a.flash.show(flashInfo, "deleted %d entities", len(ids))
	return nil
}

// afterDelete clears the marks the operation consumed and refetches, so the
// table does not keep offering rows that are gone.
func (r *entitiesResource) afterDelete(a *App) {
	if t, ok := a.table(); ok {
		t.marked = map[string]bool{}
		t.refreshNow(a.ctx)
	}
}

// describe fetches the full entity: the list payload carries only a summary,
// and relations in particular are missing from it.
func (r *entitiesResource) describe(a *App, rows []Row) error {
	entity, err := a.client.GetEntity(a.ctx, r.blueprint, rows[0].ID)
	if err != nil {
		return fmt.Errorf("cannot fetch entity %s: %w", rows[0].ID, err)
	}
	a.push(newDescribeView(a, r.blueprint+"/"+rows[0].ID, rows[0].ID, entity.Entity))
	return nil
}
