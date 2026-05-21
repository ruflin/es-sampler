# es-sampler

Sample documents from a source Elasticsearch cluster and push them into a destination cluster.

Runs in a loop: each cycle randomly samples documents from a trailing `@timestamp` window (default 24h, configurable via `--lookback`), transforms them, and bulk-pushes to the destination. Both clusters are configured via env vars or CLI flags.

## Install

Requires Go 1.24+.

```bash
go build .          # produces ./es-sampler
# or
go install .        # installs $(go env GOBIN)/es-sampler
# or via Makefile
make build          # produces bin/es-sampler
```

## Run

The simplest flow: copy `.env.example` to `.env`, fill in your source cluster
credentials, and run the binary — `.env` is auto-loaded from the current
directory.

```bash
cp .env.example .env
# edit .env, then:
./es-sampler
```

You can also pass everything via env vars or CLI flags:

```bash
# Using env vars exported in the shell
export SOURCE_ELASTICSEARCH_HOST=https://source.example.cloud.es.io:443
export SOURCE_ELASTICSEARCH_API_KEY=...
./es-sampler

# Without building first
go run . --source-host=... --source-api-key=...

# Using CLI flags
./es-sampler \
  --source-host=https://source.example.cloud.es.io:443 \
  --source-api-key=... \
  --index-pattern='logs-*' \
  --size=200 \
  --interval=5

# A custom env file
./es-sampler --env-file=./configs/prod.env
```

### How env vars are resolved

Precedence (highest first): **CLI flag > shell env var > `.env` file > default**.

`.env` is loaded from the working directory if present, or from the path given
by `--env-file`. Values in the file never override variables that are already
set in the shell/CI, so `export FOO=...` still wins.

### Recommended: enable the failure store on the destination

When sampling from many source indices — especially when funneling everything
into a single target index via `SYNC_TARGET_INDEX` / `--target-index` — mapping
conflicts are likely (e.g. the same field typed differently across sources).
Enabling the [data stream failure store](https://www.elastic.co/docs/manage-data/data-store/data-streams/failure-store)
on the destination cluster keeps those rejected documents instead of dropping
them, so you can inspect and fix mapping issues later.

**Stateful / self-managed Elasticsearch** — enable cluster-wide with a single
setting:

```json
PUT _cluster/settings
{
  "persistent": {
    "data_streams.failure_store.enabled": [
      "*"
    ]
  }
}
```

**Serverless** — the `_cluster/settings` API is not available (it returns
`api_not_available_exception`, HTTP 410). Enable the failure store per data
stream instead, via the [put data stream options](https://www.elastic.co/docs/api/doc/elasticsearch/operation/operation-indices-put-data-stream-options)
API. For example, for the data stream you sync into with
`SYNC_TARGET_INDEX` / `--target-index`:

```json
PUT _data_stream/<your-target-data-stream>/_options
{
  "failure_store": {
    "enabled": true
  }
}
```

For new data streams, you can also bake this into the matching index template
under `template.data_stream_options.failure_store.enabled: true`.

## Configuration

CLI flag > env var > default.

### Source (required)

| Env | CLI |
|---|---|
| `SOURCE_ELASTICSEARCH_HOST` | `--source-host` |
| `SOURCE_ELASTICSEARCH_API_KEY` | `--source-api-key` |

### Destination

| Env | CLI | Default |
|---|---|---|
| `ELASTICSEARCH_HOST` | — | `http://localhost:9200` |
| `ELASTICSEARCH_API_KEY` | `--dest-api-key` | — |
| `ELASTICSEARCH_USERNAME` | — | `elastic` |
| `ELASTICSEARCH_PASSWORD` | — | `changeme` |

When `ELASTICSEARCH_API_KEY` / `--dest-api-key` is set it takes precedence over username/password.

### Sync options

| Env | CLI | Default |
|---|---|---|
| `SYNC_INDEX_PATTERN` | `--index-pattern` | `logs*` |
| `SYNC_SIZE` | `--size` | `100` |
| `SYNC_INTERVAL_SECONDS` | `--interval` | `1` |
| `SYNC_LOOKBACK` | `--lookback` | `24h` |
| `SYNC_RANDOM_SEED` | `--random-seed` | — |
| `SYNC_TARGET_INDEX` | `--target-index` | preserve original |
| `SYNC_BATCH_SIZE` | `--batch-size` | same as `SYNC_SIZE` |

### Other

- `--env-file` — Path to a dotenv file to load before parsing env vars (default: `.env`; missing default file is silently ignored).
- `--no-verify-certs` — Disable TLS verification.
- `--verbose`, `-v` — Verbose logging (adds ISO timestamps).
- `--help`, `-h` — Show help.

## Kubernetes / Helm

A Helm chart lives in [`helm/`](helm/). It deploys a single-replica Deployment with `Recreate` strategy (horizontal scaling provides no benefit for a sampler loop), runs as `nonroot` with a read-only root filesystem, and installs a default-deny `NetworkPolicy` that only allows egress to DNS (53) and HTTPS (443).

### Quick start

```bash
# Create a Secret with at minimum SOURCE_ELASTICSEARCH_API_KEY and either
# ELASTICSEARCH_API_KEY or ELASTICSEARCH_USERNAME + ELASTICSEARCH_PASSWORD.
kubectl create secret generic es-sampler-secrets \
  --from-literal=SOURCE_ELASTICSEARCH_API_KEY=... \
  --from-literal=ELASTICSEARCH_API_KEY=...

helm install es-sampler ./helm \
  --set envFromSecret=es-sampler-secrets \
  --set env.SOURCE_ELASTICSEARCH_HOST=https://source.example.cloud.es.io:443 \
  --set env.ELASTICSEARCH_HOST=https://destination.example.cloud.es.io:443
```

Or with a values file:

```yaml
# my-values.yaml
envFromSecret: es-sampler-secrets

env:
  SOURCE_ELASTICSEARCH_HOST: https://source.example.cloud.es.io:443
  ELASTICSEARCH_HOST: https://destination.example.cloud.es.io:443

sync:
  size: 1000
  intervalSeconds: 60
  lookback: 10m
```

```bash
helm install es-sampler ./helm -f my-values.yaml
```

Render locally without installing:

```bash
make chart-template HELM_ARGS="-f my-values.yaml"
```

### Key values

Defaults mirror the binary's CLI defaults (`es-sampler --help`) so the chart's behaviour matches the standalone binary out of the box. Override anything per-deployment.

| Value | Default | Maps to |
|---|---|---|
| `image.repository` | `ghcr.io/ruflin/es-sampler` | — |
| `image.tag` | _(Chart.AppVersion)_ | — |
| `replicaCount` | `1` | Do not increase — duplicates work |
| `env` | `{}` | Plain key=value env vars (e.g. cluster URLs) |
| `envFromSecret` | `""` | Name of an existing Secret with API keys |
| `sync.indexPattern` | `logs*` | `SYNC_INDEX_PATTERN` |
| `sync.size` | `100` | `SYNC_SIZE` |
| `sync.intervalSeconds` | `1` | `SYNC_INTERVAL_SECONDS` |
| `sync.lookback` | `24h` | `SYNC_LOOKBACK` |
| `sync.targetIndex` | `""` | `SYNC_TARGET_INDEX` |
| `sync.batchSize` | `""` | `SYNC_BATCH_SIZE` (defaults to `sync.size`) |
| `sync.requestTimeout` | `""` | `SYNC_REQUEST_TIMEOUT` (defaults to 30s) |
| `verbose` | `false` | `--verbose` |
| `noVerifyCerts` | `false` | `--no-verify-certs` |
| `networkPolicy.enabled` | `true` | Deny-all ingress, egress 53 + 443 only |
| `extraObjects` | `[]` | Extra manifests rendered alongside the chart |

The full list with comments is in [`helm/values.yaml`](helm/values.yaml).

## Development

Common tasks are wrapped in the [Makefile](Makefile); run `make help` for the
full list.

```bash
make build        # bin/es-sampler
make test         # go test -race ./...
make lint         # go vet + gofmt check
make fmt          # gofmt -w
make tidy         # go mod tidy
make check        # lint + test + build (what CI runs, alongside tidy-check)
make run ARGS="--help"
make chart-lint   # helm lint helm/
make chart-template HELM_ARGS="-f my-values.yaml"
```

CI runs `make tidy-check`, `make lint`, `make test`, and `make build` on every
push and pull request. See [`.github/workflows/ci.yml`](.github/workflows/ci.yml).
