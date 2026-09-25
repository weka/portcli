# CLAUDE.md — portcli

## Project Overview
CLI tool for Port.io self-service actions. Built in Go with Cobra CLI framework.

## Rules
- On each code change, update CLAUDE.md and README.md if needed
- Run `/simplify` on each code change
- Nothing under `internal/` writes to stdout or stderr — errors are returned.
  The upcoming TUI owns the terminal, and one stray print corrupts the screen.
- Dependencies run one way: `cmd → {portfmt, upsert, bulk, client, config}`.
  Nothing under `internal/` may import `cmd`, because `cmd` will import the TUI
  package. Shared helpers belong in `internal/portfmt` (pure formatting and
  parsing) or a domain package, never in `package cmd`.
- Shared packages take what they need as parameters. A cobra flag variable is
  package-global mutable state in `cmd`, and reading one from `internal/` makes
  the behaviour depend on which command last parsed flags.

## TUI notes
- Entry: `tui.Run(ctx, client, opts)`. It owns the screen and returns only once
  it is restored, so a caller's `os.Exit` can never fire while tcell holds raw
  mode. It also recovers panics and re-panics *after* `Fini` — tview only
  recovers on its own goroutine, not one a view spawned.
- Adding a resource is one file: a type with `Kind`/`ID`/`Title`/`Columns`/
  `List`/`Ops`, plus an `init` calling `Register`. See `res_blueprints.go`.
  Implement `dynamicColumns` instead when the columns depend on fetched data.
- `Resource` is tabular and pure — no tview, no UI decisions — which is why
  `List` and `Columns` are testable. `View` is anything the stack can show, so
  logs and describe are not forced into a table shape they do not fit.
- **Read the rules at the top of `refresh.go` before touching background work.**
  `QueueUpdateDraw` blocks until the main loop drains it, and the main loop runs
  the key handlers — so a `stop()` that waited for its goroutine would deadlock
  with no stack trace. Only the top of the stack polls.
- The focus guard in `keys.go` is not optional: `SetInputCapture` runs before
  the focused primitive, so without it `d` typed into a field would trigger
  Describe and `:` would be untypeable.
- `tview.Table` binds `h`/`l` to column scrolling with no guard, so both are
  stolen globally (`stolenKeys`) to free `l` for logs; arrows still scroll.
  A test asserts no resource binds a key Table already owns.
- The client secret is never rendered; the client id is masked. There is a test.

## Verifying a change did not alter CLI behaviour
`scripts/compare_cli.sh [ref]` builds `ref` (default `HEAD`) in a throwaway git
worktree and runs both binaries back-to-back over ~20 invocations, diffing
stdout, stderr and exit code. Running them back-to-back matters: entities and
action runs change under you, so comparing against output captured earlier
reports live data drift as a regression.

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
    ├── config/
    │   └── config.go              # Credential loading (env vars → ~/.portcli/config.json)
    ├── portfmt/                   # Port values ⇄ display strings, and the CLI
    │   ├── portfmt.go             #   spellings of those values. Pure, no network.
    │   └── parse.go
    ├── upsert/
    │   └── upsert.go              # UPSERT_ENTITY verification (Port keeps no run record)
    ├── bulk/
    │   └── bulk.go                # Bounded-concurrency fan-out over entity ids
    └── tui/                       # tview TUI — entry point: tui.Run(ctx, client, opts)
        ├── app.go                 #   App shell, widget tree, view stack
        ├── resource.go            #   Resource interface + kind registry
        ├── view.go                #   View interface (a view need not be tabular)
        ├── table.go               #   Generic table over any Resource
        ├── refresh.go             #   Background polling — read the rules in here first
        ├── keys.go                #   Key dispatch + the focus guard
        ├── prompt.go / command.go #   ":" palette and "/" filter; parsing is pure
        ├── header.go / flash.go   #   Context panel; status line + message history
        ├── res_*.go               #   One file per resource: blueprints/entities/actions/runs
        ├── describe.go            #   JSON pane
        ├── inputs.go / logs.go    #   Action input schema; incremental log tail
        ├── formspec.go            #   Action schema → form spec (pure, main test target)
        ├── form.go                #   The run form
        ├── runwatch.go            #   Status card + live logs
        ├── upsertverify.go        #   Did the UPSERT_ENTITY action write its entity?
        ├── editentity.go          #   $EDITOR round trip + property diff
        ├── confirm.go / help.go   #   Confirmation modal; generated key map
        └── state.go / style.go    #   ~/.portcli/tui.json; theme
```

## Key Dependencies
- `github.com/spf13/cobra` — CLI framework
- `github.com/rivo/tview` — TUI widgets (what k9s uses)
- `github.com/gdamore/tcell/v2` — terminal backend behind tview
- `golang.org/x/term` — TTY detection for the bare-invocation path

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
| `tui` | | Open the interactive TUI. Also what a bare `portcli` does on a terminal; with either end redirected, bare `portcli` prints help and exits 0. Flags: `--refresh`, `--view` |
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
- **Access tokens expire and nothing announces it.** `POST /v1/auth/access_token`
  returns a bearer token (`expiresIn` ≈ 1.7h) with no expiry field on the token
  itself and no refresh endpoint. A one-shot CLI run never outlives one; a TUI
  session does. `doRequest` treats a single 401 as an aged-out token,
  re-authenticates once and replays the request; a second 401 is reported as a
  credentials error. `client.IsUnauthorized` recognises both.

## Concurrency and cancellation
- The client is safe for concurrent use. `token` is guarded by `authMu`, held
  across the authentication round-trip so a burst of concurrent requests costs
  one authentication rather than one each.
- Every exported client method takes a `context.Context` as its first argument.
  It comes from `cmd.Context()`, which `Execute` wires to a
  `signal.NotifyContext` — so `^C` aborts an in-flight request instead of
  killing the process mid-PATCH, and the process exits 130.
- `PollEntity` and `WaitForRun` `select` on `ctx.Done()` rather than
  `time.Sleep`, so an interrupt lands immediately instead of one poll interval
  later. They are still blocking loops that report nothing until they finish,
  which suits `action run --wait` and `entity wait`; a UI should poll with its
  own ticker over the single-shot `GetEntity`/`GetActionRun` instead.
- When replaying a request after re-authentication, the body is marshalled once
  and a fresh reader built per attempt. Reusing the reader sends an empty body
  the second time, which the API accepts — a PATCH would silently write nothing.

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
