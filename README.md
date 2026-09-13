# speedtest-tracker

A single-binary internet speed tracker: Ookla, Cloudflare and iperf3 targets on
named schedules, SQLite as the source of truth, with an embedded React UI.

## Status

Milestone 1 (skeleton): configuration, SQLite store and migrations, settings
store, HTTP router with `/healthz`, and the embedded SPA shell. Test engines
arrive in milestone 2.

## Requirements

- Go 1.25+ (toolchain auto-download handles patch versions)
- Node.js for building the web UI

## Quick start

```bash
make build
./speedtest-tracker
# http://localhost:8080
```

## Configuration

Only two settings come from the environment; everything else is edited in the UI
and stored in the database.

| Variable | Default | Meaning |
| --- | --- | --- |
| `ST_DB_PATH` | `/data/speedtest.db` in a container, else `./data/speedtest.db` | SQLite file |
| `ST_LISTEN` | `:8080` | HTTP listen address |

## Docker

```bash
docker run -d --name speedtest-tracker \
  -p 8080:8080 -v speedtest-data:/data \
  ghcr.io/metril/speedtest-tracker:latest
```

## Development

```bash
make dev    # Go server on :8080, Vite dev server on :5173 (proxies /api)
make test   # go vet + go test -race + vitest
make lint
```

The Vite build writes into `internal/web/dist`, which the Go binary embeds, so
`make build` must build the UI before the binary.

## License

MIT
