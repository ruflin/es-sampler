# AGENTS.md

Guidance for AI coding agents (Cursor, Claude Code, etc.) working in this repository.

## What this project is

`es-sampler` is a small Go CLI that samples documents from a source Elasticsearch cluster and pushes them into a destination cluster. See [README.md](README.md) for user-facing docs.

## Layout

```
main.go              # entry point (kept thin; loads .env then calls sampler.Run)
internal/sampler/    # all library code + tests
  config.go          # Config struct, ErrHelpRequested
  cli.go             # argv parser + env fallback, ParseConfig
  dotenv.go          # LoadDotEnv — reads .env without overriding existing env vars
  logger.go          # Logger type
  client.go          # ES client factories (unexported) + pingCluster
  sync.go            # search, transform, bulk upload, exported Run loop
  *_test.go          # table-driven tests (no external fixtures)
Dockerfile           # multi-stage: chainguard/go builder → chainguard/static runtime
.dockerignore
helm/                # self-contained Helm chart
  Chart.yaml
  values.yaml        # defaults mirror the binary's CLI defaults
  templates/         # deployment, networkpolicy, extra-manifests, NOTES.txt
  ci/                # example-values.yaml used by `make chart-template` and CI
```

Keep this layout. New Go functionality goes in `internal/sampler/` in a focused file; `main.go` stays thin. Chart changes go in `helm/` and must keep `helm lint` + `helm template -f helm/ci/example-values.yaml` clean.

## Language & style

- Go 1.24+. Format with `gofmt` (enforced by CI). Vet with `go vet`.
- Line length is not strict, but wrap long docstrings around ~100 chars.
- Prefer standard library. The only direct dep is `github.com/elastic/go-elasticsearch/v8` — think twice before adding more.
- Use `context.Context` for anything that does I/O, and respect cancellation in loops.
- Error wrapping: `fmt.Errorf("…: %w", err)`. Don't swallow errors silently except where the existing code intentionally logs and continues (bulk errors, per-cycle search failures).
- Avoid comments that just narrate code. Comments should explain *why* or call out non-obvious behavior.

## Testing

- Run `make test` (or `go test ./...`).
- Tests are colocated in `internal/sampler/*_test.go`. No external Elasticsearch is required — tests cover pure logic (CLI validation, dotenv parsing, document transform, backing-index helper).
- Add tests when you add or fix behavior. Table-driven tests are the default style.
- Use `t.Setenv` for env-var tests and `t.TempDir` / `chdirTemp(t)` for filesystem tests.

## Common workflows

Use the Makefile:

```bash
make build           # builds bin/es-sampler
make test            # go test -race ./...
make lint            # go vet + gofmt -l (fails on diffs)
make fmt             # gofmt -w
make check           # lint + test + build
make run             # go run . (pass ARGS=...)
make tidy            # go mod tidy
make image           # docker build (override IMAGE=... or IMAGE_TAG=...)
make push            # docker push (login to ghcr.io first)
make chart-lint      # helm lint helm/
make chart-template  # helm template -f helm/ci/example-values.yaml (append HELM_ARGS=...)
make clean
```

CI workflows:

- `.github/workflows/ci.yml` runs `tidy-check`, `lint`, `test`, `build`, and the chart lint/template job on every push and PR.
- `.github/workflows/image.yml` builds the multi-arch container image on every PR (build-only) and publishes to `ghcr.io/ruflin/es-sampler` on `main` and on `v*.*.*` tags.

## Compatibility

Log line formats are intentionally stable — e.g. `[source] Connected: <name> (<version>)` and `Cycle N: pushed M documents`. Preserve them when touching `sync.go` / `client.go`. Flag names and env-var precedence (CLI > env > default) should also stay consistent across releases.

When adding a new `SYNC_*` env var / CLI flag, also surface it in:

- `.env.example` (with a comment).
- `internal/sampler/cli.go` `HelpText` + the sync-options table in `README.md`.
- `helm/values.yaml` under `sync:` and `helm/templates/deployment.yaml` (use `{{- with }}` so omitted values don't render an env entry).

## Config / secrets

- There's a `.env.example` at the repo root — copy to `.env` for local dev. Do **not** commit `.env`.
- `main.go` auto-loads `.env` (or `--env-file=PATH`) via `sampler.LoadDotEnv` before `ParseConfig` runs. Existing shell/CI env vars always win over the file.
- Secrets only come from env vars or CLI flags; never hardcode them.

## Container image and Helm chart

- The Dockerfile is multi-stage: `cgr.dev/chainguard/go` builder produces a static `CGO_ENABLED=0` binary, `cgr.dev/chainguard/static` runs it as `nonroot`. Both base images are pinned by digest. Refresh by pulling latest and updating the digests in the Dockerfile.
- Chart `values.yaml` defaults match the binary's CLI defaults (principle of least surprise). Consumers override per-deployment; do not bake environment-specific values into the chart.
- The chart deliberately has no `values.production.yaml` and no dependency on any custom CRDs. Internal consumers attach environment-specific manifests (Vault SecretClaim, etc.) via `extraObjects`.
- `helm/Chart.yaml` `version` follows SemVer independently of `appVersion`. Bump `version` on every chart-only change; bump `appVersion` when wiring up a new app release.
- Chart distribution (gh-pages vs OCI) is intentionally deferred — track in the corresponding open issue before wiring up a publishing workflow.

## PRs

- Prefix commits with conventional-commit types (`feat:`, `fix:`, `refactor:`, `docs:`, `chore:`, `test:`).
- Keep PRs focused. Update `README.md` and this file when user-visible behavior or layout changes.
- Do not add a license header to new files.
