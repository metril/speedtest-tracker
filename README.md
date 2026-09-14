# speedtest-tracker

A single-binary internet speed tracker: Ookla, Cloudflare and iperf3 targets on
named schedules, SQLite as the source of truth, with an embedded React UI.

## Status

Milestone 5 (dashboard): the `Engine` interface plus the Ookla, Cloudflare,
iperf3 and fake engines; targets and schedules with cron-driven execution; a
React UI for targets, schedules and results, including a live view of an
in-progress run, a per-target dashboard, an outage timeline, CSV export and
tag management.

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

## Schedules

A schedule is a name, a cron expression, a timezone and an **ordered** list of
targets. Targets in a run execute one after another within a lane, so a
schedule's order is the order the tests run in.

| Field | Notes |
| --- | --- |
| `cron` | Standard 5-field syntax (`*/15 * * * *`, `0 3 * * *`) or a descriptor (`@hourly`, `@daily`, `@every 30m`). Seconds are not accepted. |
| `timezone` | Any IANA zone (`Europe/Zurich`). Defaults to UTC. |
| `enabled` | Disabled schedules keep their configuration but are not registered with cron. |

Behaviour:

- A cron fire only **enqueues** a run; the HTTP API and the UI never block on a
  running test.
- If the schedule's previous run is still queued or running, or the lane queue
  is full, the fire is recorded as a `skipped` run row instead of piling up.
- Missed fires during downtime are never backfilled.
- Saving a schedule returns `warnings[]` when another enabled schedule sharing a
  lane fires within 60 seconds of it in the next 24 hours — overlapping tests
  skew each other's results.

Endpoints:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/schedules` | List schedules with their next fire time |
| `POST` | `/api/v1/schedules` | Create; returns `{schedule, warnings}` |
| `GET/PUT/DELETE` | `/api/v1/schedules/{id}` | Read, update, delete |
| `POST` | `/api/v1/schedules/{id}/run` | Run the schedule's targets now (manual trigger) |
| `GET` | `/api/v1/schedules/{id}/next` | Next five fire times |
| `POST` | `/api/v1/schedules/validate` | Validate a cron expression and preview its next runs |
| `GET` | `/api/v1/runs?schedule_id=N` | Runs of one schedule |

## Live run view

Every run streams `progress`, `result` and `run` events over
`GET /api/v1/events` (SSE, coalesced to 10 Hz per result). The UI shows a speed
gauge, a throughput sparkline, ping/jitter/loss tiles, the server and ISP, a
per-target stepper for multi-target runs and a cancel button. Runs started from
the UI open the panel directly; scheduled runs appear as a compact bar.

## Dashboard

The dashboard shows one card per target — latest download, upload and ping, a
24-hour sparkline, a status dot and a "Run now" button — over a 24h / 7d / 30d
range selector, plus summary tiles (tests run, success rate, average download,
worst ping), a download/upload chart, a ping/jitter chart and an outage strip.

Chart data is downsampled **in SQL**: `GET /api/v1/targets/{id}/history?range=7d`
returns `{bucket_seconds, points[]}` with at most ~500 buckets, each carrying
avg/min/max download, upload and ping plus the test and failure counts for that
bucket. `GET /api/v1/stats/summary?range=` is cached in-memory for 30 seconds
and sent with `Cache-Control: max-age=30`.

## Outages

`GET /api/v1/outages?from&to&gap_seconds` returns the trouble timeline:
consecutive failed or degraded results for one target that are no further apart
than `gap_seconds` (default 1800, i.e. twice a 15-minute schedule) collapse into
one incident with its start, end and test count; every `skipped` cron fire is its
own incident named after the schedule.

## CSV export

`GET /api/v1/results.csv` accepts the same filters as `GET /api/v1/results`
(`target_id`, `engine`, `status`, `from`, `to`, `tag`) and streams rows as they
are read from SQLite, so exporting a year of history costs one row of memory.
The Results page's "Export CSV" button links to it with the filters currently
applied.

## Theme

The UI ships light, dark and system themes; the header toggle persists the
choice in `localStorage` (`st-theme`) and stamps `light`/`dark` on `<html>`.
Every colour comes from one token set defined in `web/src/index.css`, including
the shared chart palette.

## API conventions

`/api/v1` follows a few consistent shapes:

- Paginated lists (`/results`, `/runs`) return `{"<plural>": [...], "next_cursor": "..."}`;
  pass `next_cursor` back as `?cursor=` to fetch the next page, and an empty
  string means the listing is exhausted.
- Every other list (`/targets`, `/tags`, `/ookla/servers`) returns a bare JSON array.
- Errors always use the envelope `{"error": {"code": "...", "message": "..."}}`,
  with a matching HTTP status code (400 invalid_request, 404 not_found, 500 internal_error, etc).
- `/results`, `/targets`, `/schedules` and `/runs` send an `ETag`; a matching
  `If-None-Match` gets back `304 Not Modified` with no body.

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
