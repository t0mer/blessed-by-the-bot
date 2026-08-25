# blessed-by-the-bot

Self-hosted WhatsApp bot that sends automated blessings — birthdays, wedding
anniversaries and custom events — to your contacts on schedule, and joins in
when a watched group starts congratulating someone.

Single Go binary with an embedded web UI. No external database, no separate
frontend container.

> **Status:** ground-up Go rewrite in progress. The scheduler, WhatsApp
> providers and web UI land in subsequent phases; this revision ships the
> server skeleton.

## Quick start

```bash
# Generate an encryption key for provider credentials at rest.
export BBTB_ENCRYPTION_KEY="$(go run ./cmd/blessedbot genkey)"

make build
./bin/blessedbot
```

Open <http://localhost:8080>.

### Docker

```bash
docker run -d \
  --name blessedbot \
  -p 8080:8080 \
  -e BBTB_ENCRYPTION_KEY="$(docker run --rm techblog/blessed-by-the-bot:latest genkey)" \
  -v blessedbot-data:/data \
  techblog/blessed-by-the-bot:latest
```

Or use the provided `docker-compose.yml`. Keep the `blessedbot-data` volume: it
holds the SQLite database, including your encrypted provider credentials.

The image is built `FROM scratch`, runs as uid `65532`, and is published for
`linux/amd64`, `linux/arm64` and `linux/arm/v7`.

## Configuration

Precedence: **command-line flags > environment variables > YAML config file > defaults.**

| Flag | Environment variable | Default | Description |
|---|---|---|---|
| `--config` | `BBTB_CONFIG` | `./config.yaml` if present | Path to the YAML config file |
| `--port` | `BBTB_PORT` | `8080` | HTTP listening port |
| `--data-dir` | `BBTB_DATA_DIR` | `./data` (`/data` in Docker) | SQLite database and state |
| `--log-level` | `BBTB_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `--dev` | — | `false` | Verbose text logs; encryption key not required |
| — | `BBTB_ENCRYPTION_KEY` | *(required)* | Base64-encoded 32-byte AES-256 key |

`BBTB_ENCRYPTION_KEY` is environment-only by design — it is never a flag and is
never read from the config file. Without it the server refuses to start unless
`--dev` is passed. Generate one with `blessedbot genkey`.

Runtime behaviour (WhatsApp provider, send times, group-echo thresholds) is
configured in the web UI and stored in the database, not in the config file.
See `config.example.yaml`.

## Commands

| Command | Purpose |
|---|---|
| `blessedbot` | Run the server |
| `blessedbot version` | Print the build version |
| `blessedbot genkey` | Generate a `BBTB_ENCRYPTION_KEY` value |
| `blessedbot healthcheck --url <url>` | Probe `/healthz`; exits non-zero when unhealthy |

`healthcheck` exists because the `scratch` image has no shell and no `curl` —
the container healthcheck invokes the binary itself.

## Endpoints

| Path | Purpose |
|---|---|
| `/healthz` | Liveness probe — `{"status":"ok","version":"…"}` |
| `/api/v1/*` | JSON API |
| `/*` | Embedded single-page web UI |

API errors use a consistent envelope:

```json
{ "error": { "code": "not_found", "message": "resource not found" } }
```

## Development

```bash
make lint    # go vet + golangci-lint
make test    # go test ./...
make build   # bin/blessedbot
make run     # go run with --dev
make docker  # build the scratch image
```

Everything ships with `CGO_ENABLED=0`. The race detector (`make test-race`)
is the one exception — it requires cgo.

## License

Apache-2.0 — see [LICENSE](LICENSE).
