# Repository Guidelines

## Project Structure & Module Organization
`main.go` hosts the CLI entry point; reusable services live in `internal/` (`proxy`, `monitor`, `tracking`, `web`, `tui`). Configuration templates are under `config/` and test fixtures under `config.test/` and `endpoint.test`. Automation and release helpers sit in `scripts/`, while reference docs live in `docs/`. Tests are grouped inside `tests/` (unit, integration) with shared data in `tests/testdata`. Runtime state such as SQLite stores belongs in the git-ignored `data/` directory.

## Build, Test, and Development Commands
- `go build ./...` or `go build -o cc-forwarder` compiles the service.
- `go run ./main.go -config config/config.yaml` exercises config changes without a binary.
- `./scripts/build.sh vX.Y.Z` mirrors the release matrix and archives assets in `dist/`.
- `./run_tests.sh` runs the suspend-flow regression pack and leaves logs in `test_reports/`.
- `go test ./internal/... ./config/...` offers a fast loop for touched packages.

## Coding Style & Naming Conventions
Stick to Go 1.23 tooling: run `gofmt` or `go fmt ./...`, organise imports standard/third-party/local, and prefer short package names with `camelCase` identifiers (`SSEStream`). YAML keys stay lower_snake_case following `config/example.yaml`. Static assets remain under `internal/web/static`; include a brief comment when adding bundled JS or CSS.

## Testing Guidelines
Place new unit specs under `tests/unit/<module>` and name functions `TestSomething`. Integration scenarios belong in `tests/integration/request_suspend/` and should consume the sample configs already checked into `config.test/`. Extend `run_tests.sh` if you add a new stage so CI and local smoke runs stay aligned. For coverage-sensitive work, run `go test -cover ./internal/...` and attach relevant `test_reports/*.log` snippets to bug reports or PR discussions.

## Commit & Pull Request Guidelines
Commit messages follow Conventional Commits (`feat:`, `fix:`, `refactor:`) with concise, imperative subjects; Chinese context is welcome when it improves clarity. Before opening a PR, sync with the latest `main`, ensure `go test ./...` and `./run_tests.sh` succeed, and note config or schema changes (`migrate_timezone.sql`, `data/usage.db` expectations) in the description. Link the associated issue, list manual verification steps, and capture screenshots when touching the web console.

## Security & Configuration Tips
Do not commit live Claude tokens—use `config/example.yaml` as a template and keep secrets in ignored overrides. Sanitize `data/usage.db` before sharing archives because it records request telemetry. When exposing the web UI, restrict `web.host` to trusted subnets and front the service with TLS termination as documented under `docs/`.
