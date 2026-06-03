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
│   ├── action.go                  # `action` parent command
│   ├── run.go                     # `action run <action-id>` — execute action, optional wait/poll
│   ├── status.go                  # `action status <run-id>` — get action run status
│   └── logs.go                    # `action logs <run-id>` — get action run logs
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
| `entity get` | `<blueprint> [entity-id]` | Fetch entity (full JSON), or list entities as table (identifier, status, owner, created, TTL) if no ID given. Flags: `--property`/`-p` (print single property value), `--filter`/`-f` (filter by field=value) |
| `entity update` | `<blueprint> [entity-id] [key=value ...]` | Update entity properties. Flags: `--json` (JSON object), `--all` |
| `entity delete` | `<blueprint> [entity-id]` | Delete entity. Flags: `--all`, `--yes`/`-y`, `--filter`/`-f` (with --all, filter by field=value) |
| `action run` | `<action-identifier>` | Execute action. Flags: `--input`, `--wait`, `--poll`, `--timeout`, `--run-as`, `--entity`, `--id` |
| `action status` | `<run-id>` | Get action run status |
| `action logs` | `<run-id>` | Get action run logs |

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
