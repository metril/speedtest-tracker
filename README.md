# speedtest-tracker

A single-binary internet speed tracker: Ookla, Cloudflare and iperf3 targets on
named schedules, SQLite as the source of truth, with an embedded React UI.

## Status

Milestone 2 (engines): the `Engine` interface plus the Ookla, Cloudflare,
iperf3 and fake engines with fixture-tested parsers. Targets, schedules and
the UI arrive in later milestones.

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

## Engines

| Engine | How it runs | Options |
| --- | --- | --- |
| `ookla` | `speedtest -f jsonl --progress=yes --accept-license --accept-gdpr [-s ID]`; server list from `speedtest -L -f json`, cached | `server_id` |
| `cloudflare` | native Go against `speed.cloudflare.com` (`/cdn-cgi/trace`, `/__down`, `/__up`), p90 of per-transfer throughput | `download_sizes`, `upload_sizes`, `latency_samples`, `base_url` |
| `iperf3` | `iperf3 -c host -p port -J` (or `--json-stream` on 3.17+) | `host`, `port`, `protocol`, `reverse`, `bidir`, `parallel`, `duration_s`, `udp_bitrate`, `bind`, `username`, `password`, `rsa_public_key_path` |
| `fake` | deterministic, no I/O; used by tests | `fail`, `download_bps`, `upload_bps` |

Binary paths and the Ookla consent flags live in the Engines settings section
(`engines.speedtest_bin`, `engines.iperf3_bin`, `engines.ookla_accept_license`,
`engines.ookla_accept_gdpr`, `engines.server_list_ttl_seconds`).

Run one test by hand (requires the corresponding binary to be installed):

```bash
./speedtest-tracker run --engine cloudflare --opts '{}'
./speedtest-tracker run --engine ookla --opts '{"server_id":12345}'
./speedtest-tracker run --engine iperf3 --opts '{"host":"nas.lan","reverse":true}'
```

`run` reads the same settings database as the server (`ST_DB_PATH`),
creating it with the seeded defaults if it doesn't exist yet, so binary
paths and the Ookla consent flags match whatever the server has configured.
`--speedtest-bin`/`--iperf3-bin` override just the binary path for that one
invocation. The Result JSON goes to stdout; progress events stream to
stderr as JSON lines.

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
