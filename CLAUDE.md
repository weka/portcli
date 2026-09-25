# CLAUDE.md — portcli

## Project Overview
CLI tool for Port.io self-service actions. Built in Go with Cobra CLI framework.

## Rules
- On each code change, update CLAUDE.md and README.md if needed
- Run `/simplify` on each code change

## Agentic Flow

The main agent is an orchestrator. It should delegate work via Task tool and minimize direct tool use. Direct tool use is acceptable only for 1-2 quick checks to orient. Model should always be set explicitly on tasks/subagents.

### Delegation Model (in order)

1. **Haiku task** — all codebase exploration, investigation, searching, and reading files. Even for complex debugging — haiku can read and trace code paths. It's 10-20x cheaper than opus.
2. **Sonnet task** — code edits, test runs, build verification, deploy flows.
3. **Opus task** — only for complex plan generation that requires deep understanding of the codebase. Use it only while having better initial context from a haiku task, or when sonnet is struggling with execution.

## Project Structure
```
portcli/
├── main.go                        # Entry point → cmd.Execute()
├── go.mod                         # Module: github.com/weka/portcli (Go 1.26.3)
├── .goreleaser.yaml               # GoReleaser config (cross-platform builds)
├── .semrelrc                      # Semantic release config
├── Makefile                       # Build helpers (test, build, clean)
├── .github/workflows/
│   ├── release.yaml               # Release pipeline (semrel → goreleaser → homebrew tap)
│   └── vet.yaml                   # PR validation (go vet)
├── cmd/                           # CLI commands (Cobra)
│   ├── root.go                    # Root command, registers subcommands, Version var
│   ├── blueprint.go               # `blueprint` parent command
│   ├── list_blueprints.go         # `blueprint list` — list blueprints with optional regex filter
│   ├── get_blueprint.go           # `blueprint get <identifier>` — show blueprint field names
│   ├── entity.go                  # `entity` parent command
│   ├── get_entity.go              # `entity get <blueprint> <entity-id>` — fetch entity
│   ├── update_entity.go           # `entity update <blueprint> [entity-id]` — update entity field
│   ├── delete_entity.go           # `entity delete <blueprint> [entity-id]` — delete entity
│   ├── wait_entity.go             # `entity wait <blueprint> <entity-id>` — poll until a property matches
│   ├── action.go                  # `action` parent command
│   ├── get_action.go              # `action get <action-id>` — show an action's input schema
│   ├── run.go                     # `action run <action-id>` — execute action, optional wait/poll,
│   │                              #   verifies the entity for UPSERT_ENTITY actions
│   ├── status.go                  # `action status <run-id>` — get action run status
│   ├── logs.go                    # `action logs <run-id>` — get action run logs
│   └── list_runs.go               # `action list` — list action runs with filters
├── scripts/
│   └── sign_macos.sh              # macOS codesign + notarize (runs in CI)
└── internal/
    ├── client/
    │   └── client.go              # Port.io HTTP client, OAuth auth, all API calls
    └── config/
        └── config.go              # Credential loading (env vars → ~/.portcli/config.json)
```

## Key Dependencies
- `github.com/spf13/cobra` — CLI framework

## CLI Commands
| Command | Args | Description |
|---------|------|-------------|
| `blueprint list` | | List blueprints. Flags: `--filter` (regex) |
| `blueprint get` | `<identifier>` | Show blueprint schema fields (name, title, type) |
| `entity get` | `<blueprint> [entity-id]` | Fetch entity (full JSON), or list entities as table if no ID given. Flags: `--property`/`-p` (print single property value), `--filter`/`-f` (filter by field=value), `--columns`/`-c` (comma-separated property columns to show, default: identifier only) |
| `entity update` | `<blueprint> [entity-id] [key=value ...]` | Update entity properties. Flags: `--json` (JSON object), `--all`, `--filter`/`-f` (filter by field=value, implies --all) |
| `entity delete` | `<blueprint> [entity-id]` | Delete entity. Flags: `--all`, `--yes`/`-y`, `--filter`/`-f` (with --all, filter by field=value) |
| `entity wait` | `<blueprint> <entity-id>` | Poll an entity until a property matches a regex. Flags: `--property`/`-p` (default `status`), `--for` (success regex, required), `--fail-for`, `--timeout` (default 3600), `--interval` (default 20) |
| `action get` | `<action-identifier>` | Show a self-service action's input schema. Flags: `--json`, `--enum <input>` |
| `action run` | `<action-identifier>` | Execute action. Flags: `--input`, `--wait`, `--poll`, `--timeout`, `--run-as`, `--entity`, `--id`, `--json` |
| `action status` | `<run-id>` | Get action run status |
| `action logs` | `<run-id>` | Get action run logs |
| `action list` | | List action runs as table. Flags: `--entity`/`-e`, `--blueprint`/`-b`, `--action`/`-a` (client-side), `--status`/`-s` (client-side, case-insensitive), `--limit`/`-l` (default 20), `--json` |

## Port API behaviors worth knowing

- **Entity filters must be translated.** Port's `/v1/entities/search` addresses an
  entity's top-level fields with a `$` prefix (`$identifier`, `$title`, `$createdAt`, …).
  An unprefixed name is read as an ordinary property and silently matches nothing, so
  `client.searchProperty` maps the bare names before building the rule. Every `--filter`
  on `entity get`/`update`/`delete` goes through `SearchEntitiesWithFilter`, so that one
  translation covers all of them.
- **`UPSERT_ENTITY` actions keep no run record.** Port returns a run id from
  `POST /v1/actions/{id}/runs`, then discards it — `GET /v1/actions/runs/{id}` 404s
  whether or not the upsert succeeded, and the run never appears in `action list`.
  `--wait` and `action status` are therefore useless for these actions, and the entity
  is the only evidence. `action run` detects this backend and verifies the target entity
  instead (`verifyUpsert` in `cmd/run.go`).
- **A required blueprint property that resolves empty silently drops the upsert.** The
  common cause is a hidden action input defaulted from a jqQuery such as `.user.email`,
  which resolves to nothing under client-credential auth. `unresolvedRequired` reports
  exactly which ones did not resolve.
- **`GET /v1/actions/runs/{id}/logs` answers 200 with an empty list** for a run that
  does not exist, so an empty result is ambiguous rather than proof of no output.
  It accepts `offset`/`limit`, so a repeated poll can tail incrementally —
  `GetRunLogsFrom` takes the number of lines already seen.
- **An action input's `enum` may be an object, not a list.** Port sends either the
  choices themselves or `{"jqQuery": "…"}` for an enum it evaluates server-side.
  `ActionInput.Enum` is therefore `json.RawMessage`, read through `EnumValues()`
  (choices, or ok=false) and `DynamicEnum()`. Decoding it into `[]any` failed the
  *entire* action fetch, and because `verifyUpsert` reads any `GetAction` error as
  "not an upsert", that silently disabled UPSERT_ENTITY verification for every
  affected action. `default` and `visible` take the same jq form — `visible`
  especially (most inputs that have it use a query), which is why a UI should show
  such inputs rather than try to evaluate their visibility.
- **`GET /v1/actions?trigger_type=self-service&version=v2` returns complete action
  objects**, trigger inputs and invocation mapping included — so `ListActions`
  answers in one call what would otherwise be a `GetAction` per action. Pass
  `version` explicitly; the trigger shape differs between versions.

## Configuration
- Env vars: `PORT_CLIENT_ID`, `PORT_CLIENT_SECRET`, `PORT_BASE_URL`
- Fallback: `~/.portcli/config.json`
- Default base URL: `https://api.getport.io`

## Build & Run
```bash
go build -o portcli .
./portcli <command> [flags]
```

## Release & Distribution
- **Versioning**: `cmd.Version` set via ldflags at build time (`-X github.com/weka/portcli/cmd.Version=...`)
- **Semantic release**: `.semrelrc` + `go-semantic-release/action` — auto-bumps version from conventional commits
- **GoReleaser**: `.goreleaser.yaml` — builds linux/darwin × arm64/amd64 binaries
- **Homebrew**: `brew tap weka/portcli && brew install portcli`
  - Tap repo: `weka/homebrew-portcli` (auto-updated by release workflow)
  - Binaries hosted on gh-pages branch
- **macOS signing**: `scripts/sign_macos.sh` — codesign + notarize via Vault-stored Apple certs
- **CI**: `.github/workflows/release.yaml` (release → sign → tap on push to main), `.github/workflows/vet.yaml` (go vet on PRs)
