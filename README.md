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

Options:
- `--property`, `-p` — print only the value of a specific property
- `--filter`, `-f` — filter entities by property value (format: field=value)

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
- `--filter`, `-f` — filter entities by property value (format: field=value), requires `--all`

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
