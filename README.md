# portcli

CLI tool for [Port.io](https://www.getport.io/) self-service actions.

## Installation

### Homebrew (macOS)

```bash
brew tap weka/portcli
brew install portcli
```

### From source

```bash
go build -o portcli .
```

## Configuration

Set credentials via environment variables:

```bash
export PORT_CLIENT_ID="your-client-id"
export PORT_CLIENT_SECRET="your-client-secret"
export PORT_BASE_URL="https://api.getport.io"  # optional, this is the default
```

Or create `~/.portcli/config.json`:

```json
{
  "client_id": "your-client-id",
  "client_secret": "your-client-secret",
  "base_url": "https://api.getport.io"
}
```

## Usage

### Interactive TUI

```bash
portcli          # on a terminal, opens the TUI
portcli tui      # the same, explicitly
```

A k9s-style interface over the catalog: browse blueprints, entities, actions and
action runs, run an action from a form built out of its input schema, watch its
logs stream, and edit or delete entities.

Press `:` for the command palette (with completion), `/` to filter what is on
screen, and `?` for the full key map. The view you were last in is remembered in
`~/.portcli/tui.json` and reopened next time; delete that file to start from the
blueprint list again.

| Key | |
|---|---|
| `:` | command palette — `blueprints`, `entities <blueprint>`, `actions`, `runs` |
| `/` | filter the rows on screen (regex, case-insensitive) |
| `enter` | drill into the selected row |
| `d` | describe — the full object as JSON |
| `l` | logs (on a run) · an entity's runs (on an entity) |
| `r` | run the selected action |
| `e` | edit an entity's properties in `$EDITOR` |
| `ctrl-d` | delete (asks first) |
| `space` / `ctrl-a` / `ctrl-\` | mark a row / mark all / clear marks — operations apply to marks |
| `ctrl-r` / `ctrl-w` | refresh now / show wide columns |
| `esc` | clear the filter, else go back |
| `q` | back, or quit from the top |

Useful palette commands: `:entities <blueprint> status=Failed` filters
server-side; `:cols status,owner` overrides the auto-chosen entity columns;
`:refresh 10s` changes this view's poll interval; `:errors` lists recent
messages, including background refreshes that failed.

Editing an entity opens its properties as JSON in `$EDITOR` (`$VISUAL` first,
then `vi`) and sends only the properties that changed. Emptying the file cancels;
saving something that is not valid JSON reopens the editor with the parse error
at the top rather than discarding the edit.

Actions that Port carries out itself by writing an entity (`UPSERT_ENTITY`) get a
verification screen instead of a run watcher, because Port keeps no run record
for them — see [Actions that create an entity](#actions-that-create-an-entity).
If the entity does not appear, the screen names the required properties that
resolved empty and offers to reopen the form with your answers still filled in.

`portcli` with its output redirected (`portcli | cat`, CI) has no terminal to
draw on and prints this help instead, exiting 0, exactly as it did before the TUI
existed. `portcli tui` in that situation is an error, since it was asked for by
name. `portcli --help` and `portcli --version` are unaffected.

Options for `portcli tui`:
- `--refresh <duration>` — override the per-resource poll intervals
- `--view <view>` — open this view instead of the remembered one, e.g. `--view "entities my-blueprint"`

### Blueprint commands

#### List blueprints

```bash
portcli blueprint list
```

Filter by regex pattern:

```bash
portcli blueprint list --filter "^service"
portcli blueprint list --filter "deploy|build"
```

Options:
- `--filter` — regex pattern to filter blueprint identifiers

#### Get blueprint fields

```bash
portcli blueprint get <identifier>
```

Displays the blueprint's schema properties in a table with columns: Field (key), Title, and Type, sorted alphabetically by field key.

### Entity commands

#### Get a catalog entity

```bash
portcli entity get <blueprint> [entity-identifier]
```

If no entity identifier is provided, lists all entities for the blueprint:

```bash
portcli entity get <blueprint>
```

Get a specific property value:

```bash
portcli entity get <blueprint> <entity-identifier> --property version
portcli entity get <blueprint> <entity-identifier> -p status
```

Filter entities by property value:

```bash
portcli entity get <blueprint> --filter "environment=production"
portcli entity get <blueprint> -f "status=active"
```

The filter field may also be one of an entity's top-level fields — `identifier`,
`title`, `blueprint`, `team`, `createdAt`, `updatedAt`, `createdBy`, `updatedBy`:

```bash
portcli entity get <blueprint> -f "identifier=my-entity"
```

Options:
- `--property`, `-p` — print only the value of a specific property
- `--filter`, `-f` — filter entities by property or top-level field (format: field=value)

#### Update entity properties

```bash
portcli entity update <blueprint> <entity-identifier> status=active
portcli entity update <blueprint> <entity-identifier> ttl="2026-06-01T09:13:26" status=active
```

Use `--json` for complex or nested values:

```bash
portcli entity update <blueprint> <entity-identifier> --json '{"status": "active", "metadata": {"nested": true}}'
```

Both key=value args and `--json` can be combined (--json takes precedence on conflicts).

Update all entities of a blueprint:

```bash
portcli entity update <blueprint> --all status=active
```

Options:
- `--json` — JSON object of properties to update
- `--all` — update all entities of the blueprint (uses bounded concurrency)

#### Delete an entity

```bash
portcli entity delete <blueprint> <entity-identifier>
```

Skip confirmation prompt:

```bash
portcli entity delete <blueprint> <entity-identifier> --yes
```

Delete all entities of a blueprint:

```bash
portcli entity delete <blueprint> --all
portcli entity delete <blueprint> --all --yes
```

Delete all entities matching a filter:

```bash
portcli entity delete <blueprint> --all --filter "owner=user@example.com"
portcli entity delete <blueprint> --all --filter "environment=staging" --yes
```

Options:
- `--all` — delete all entities of the blueprint (uses bounded concurrency)
- `--yes`, `-y` — skip confirmation prompt
- `--filter`, `-f` — filter entities by property or top-level field (format: field=value), requires `--all`

### Action commands

#### Execute an action

```bash
portcli action run <action-identifier> --input key1=value1 --input key2=value2
```

Options:
- `--input` — key=value pairs (supports JSON values)
- `--wait` — wait for completion
- `--poll` — polling interval in seconds (default: 2)
- `--timeout` — timeout in seconds (default: 120)
- `--run-as` — email to run as
- `--id` — entity identifier

##### Actions that create an entity

Port runs a "Create/Update entity" (`UPSERT_ENTITY`) action itself rather than
handing it to a backend, and keeps no run record for it: the run id it returns
404s immediately, whether or not the entity was created, and never shows up in
`action run status` or `action list`. `--wait` cannot help.

For these actions `portcli` verifies the target entity exists instead, and fails
if it does not — so a rejected upsert is reported rather than looking like a
successful run. A required blueprint property that resolves to an empty value is
the usual reason one is rejected, and the error names it:

```
action createOperatorTestExecution did not create entity operator_test_execution/my-run
  required operator_test_execution properties that did not resolve:
    owner — from input "owner", which resolved empty (pass --input owner=...)
```

An input whose default is a jqQuery over the calling user (e.g. `.user.email`)
resolves to nothing under client-credential auth, so pass it explicitly.

#### Check action run status

```bash
portcli action status <run-id>
```

#### Get action run logs

```bash
portcli action logs <run-id>
```

#### List action runs

```bash
portcli action list
portcli action list --entity <entity-id> --blueprint <blueprint-id>
portcli action list --entity <entity-id> --blueprint <blueprint-id> --action destroy --limit 10
portcli action list --entity <entity-id> --blueprint <blueprint-id> --status FAILURE
portcli action list --entity <entity-id> --blueprint <blueprint-id> --action destroy --json
```

Options:
- `--entity`/`-e` — filter by entity identifier
- `--blueprint`/`-b` — blueprint identifier of the entity
- `--action`/`-a` — filter by action identifier (client-side)
- `--status`/`-s` — filter by run status: IN_PROGRESS, SUCCESS, FAILURE (client-side, case-insensitive)
- `--limit`/`-l` — max runs to fetch from API (default: 20)
- `--json` — emit machine-readable JSON array
