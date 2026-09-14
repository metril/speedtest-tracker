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

Almost everything is edited in the UI and stored in the database; a handful
of environment variables configure the process itself or seed settings on
first boot.

| Variable | Default | Meaning |
| --- | --- | --- |
| `ST_DB_PATH` | `/data/speedtest.db` in a container, else `./data/speedtest.db` | SQLite file |
| `ST_LISTEN` | `:8080` | HTTP listen address (bootstrap-only: read once at startup, not a settings key) |
| `ST_LOCK_ENV` | `false` | When `true`, every `ST_<SECTION>_<KEY>` variable below becomes authoritative — see below |

### Environment variables

Any settings key can also be seeded from the environment as
`ST_<SECTION>_<KEY>` (uppercased, with `.` replaced by `_`), for example
`auth.mode` becomes `ST_AUTH_MODE`. Values are parsed as JSON, falling back
to a plain string if that fails, so booleans, numbers and arrays don't need
quoting tricks beyond normal shell escaping:

```bash
ST_AUTH_MODE=token
ST_AUTH_TRUSTED_PROXIES='["172.16.0.0/12"]'
ST_INTEGRATIONS_VM_URL=http://victoria-metrics:8428
ST_GENERAL_TIMEZONE=Europe/Zurich
```

Semantics:

- **`ST_LOCK_ENV=false` (default)** — an env value only *seeds* a key the
  database has never seen (i.e. still at its built-in default). It never
  overwrites a value already set via the UI or a previous env seed, so a
  fresh container comes up configured but the operator's later changes in
  the UI always win.
- **`ST_LOCK_ENV=true`** — every matching env value is rewritten into the
  database on every boot, authoritative over the UI. `PUT /api/v1/settings`
  rejects a write to a locked key with `400`, and the UI shows the field as
  *set by environment* and disables it.

## Authentication

Three modes, set via `auth.mode` (Settings → Auth or `ST_AUTH_MODE`):

- **`open`** (default) — no authentication; anyone who can reach the port
  has full control. The UI shows a permanent banner while this mode is
  active as a reminder that the instance is unprotected.
- **`forward_auth`** — identity comes from headers set by a reverse proxy in
  front of speedtest-tracker. Can additionally accept bearer tokens via
  *Accept API tokens as well* (`auth.allow_tokens`), which is how scripts
  reach an instance sitting behind SSO.
- **`token`** — every request needs a bearer API token.

### Forward auth

Forward auth trusts the `Remote-User` / `Remote-Groups` headers (configurable,
defaults shown) set by whatever sits in front of the app — but **only** when
the connecting peer's address is inside one of the configured trusted-proxy
CIDRs (`auth.trusted_proxies`). An empty CIDR list denies every request; that
is deliberate, since without it any client could set those headers itself
and walk straight in.

| Setting | Default | Meaning |
| --- | --- | --- |
| `auth.user_header` | `Remote-User` | Header carrying the username |
| `auth.groups_header` | `Remote-Groups` | Header carrying the user's groups |
| `auth.groups_separator` | `,` | Separator used to split the groups header |
| `auth.trusted_proxies` | `[]` | CIDRs the identity headers are trusted from |
| `auth.admin_group` | (unset) | Group required for admin access |

When `admin_group` is set, only members of that group are admins; when it is
unset, everyone who authenticates via forward auth is an admin.

**Reverse-proxy examples:**

*Traefik + Authelia* — a `forwardAuth` middleware pointed at Authelia, with
the trusted CIDR set to Traefik's own Docker network (e.g. `172.16.0.0/12`),
not the client's:

```yaml
http:
  middlewares:
    authelia:
      forwardAuth:
        address: http://authelia:9091/api/authz/forward-auth
        trustForwardHeader: true
        authResponseHeaders:
          - Remote-User
          - Remote-Groups
          - Remote-Name
          - Remote-Email
```

```yaml
# ST_AUTH_TRUSTED_PROXIES='["172.16.0.0/12"]'
```

*Authelia standalone* — an `access_control` rule for the host, and the
headers it emits on a successful auth:

```yaml
access_control:
  rules:
    - domain: speedtest.example.com
      policy: two_factor
```

Authelia emits `Remote-User`, `Remote-Groups`, `Remote-Name` and
`Remote-Email` on the proxied request.

*Caddy* — a `forward_auth` directive, copying just the headers this app
needs:

```
speedtest.example.com {
    forward_auth authelia:9091 {
        uri /api/authz/forward-auth
        copy_headers Remote-User Remote-Groups
    }
    reverse_proxy speedtest-tracker:8080
}
```

If Caddy shares the host with the app (rather than running in its own
container network), the trusted CIDR is `127.0.0.1/32` — the loopback
address Caddy connects from, not the client's.

### API tokens

Create tokens in Settings → Auth. The plaintext is shown exactly once; only
a SHA-256 digest is stored. Send it as `Authorization: Bearer <token>`.
`GET /api/v1/events` additionally accepts `?token=` because `EventSource`
cannot set headers — prefer the header everywhere else, since query strings
land in proxy access logs. Revoking the last token while in `token` mode is
refused, to avoid locking yourself out.

### Exempt endpoints

`/healthz` and `/metrics` are never authenticated, so probes and Prometheus
keep working regardless of mode. If the instance is public, put `/metrics`
behind the proxy or disable it (Settings → Integrations).

### Locked out?

Restart the container with `ST_AUTH_MODE=open` set. This env var is always
applied on boot, even if `auth.mode` was already changed to something else
in the UI, so `ST_AUTH_MODE=open` alone is enough to get back in — you do
not need `ST_LOCK_ENV=true` for this one variable. Add `ST_LOCK_ENV=true`
only if you also want the environment to keep overriding `auth.mode` (and
every other `ST_<SECTION>_<KEY>` variable you've set) on every subsequent
boot while you fix the configuration in the UI. Once the configuration is
correct, remove `ST_AUTH_MODE=open` (and `ST_LOCK_ENV=true`, if set) and
restart to switch back.

### Security notes

- Run behind TLS; credentials and tokens are sent in the clear otherwise.
- The SPA shell itself is served without authentication — it contains no
  data and calls `GET /api/v1/me` on load to decide what to render.
- Tokens are full-access in this release; there is no per-token scoping.

## Engines

| Engine | How it runs | Options |
| --- | --- | --- |
| `ookla` | `speedtest -f jsonl --progress=yes --accept-license --accept-gdpr [-s ID]`; server list from `speedtest -L -f json`, cached | `server_id` |
| `cloudflare` | native Go against `speed.cloudflare.com` (`/cdn-cgi/trace`, `/__down`, `/__up`), p90 of per-transfer throughput | `download_sizes`, `upload_sizes`, `latency_samples`, `base_url` |
| `iperf3` | `iperf3 -c host -p port -J` (or `--json-stream` on 3.17+) | `host`, `port`, `port_range_end`, `protocol`, `reverse`, `bidir`, `parallel`, `duration_s`, `udp_bitrate`, `bind`, `username`, `password`, `rsa_public_key_path` |
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

When `port_range_end` is set above `port`, a run that fails because the
server reports it is busy running another test (`the server is busy
running a test. try again later`) is retried on the next port up through
`port_range_end`, capped at 5 attempts total. Any other error, or running
out of ports, fails the run with that attempt's error. Without
`port_range_end`, a busy server fails the run outright on the single
configured port.

### Ookla server search

`GET /api/v1/ookla/servers?q=&country=&limit=` merges the local
`speedtest -L` list with a wider speedtest.net search, returning
`{"servers": [...], "near": "..."}`. A postcode-shaped `q` (e.g. `80202`,
`SW1A 1AA`) is geocoded via Nominatim (`postalcode=` scoped by
`countrycodes=` when `country` is given, falling back to an unscoped
postcode search, then a free-form `q=` search, then Open-Meteo as a last
resort) so postcodes resolve correctly instead of falling through to a
generic name search. `country`, when given, must be a 2-letter code
(case-insensitive; the server lowercases it) or the request is rejected
with 400. `near` is the resolved place's name (first two comma-separated
parts of the geocoder's result), present whenever a geocode point was
used to widen and sort the results by distance. Nominatim requests are
capped at 1/second and always carry an identifying `User-Agent`
(`speedtest-tracker/<version> (+https://github.com/metril/speedtest-tracker)`),
per its usage policy.

### Public iperf3 server list

`GET /api/v1/iperf3/servers?q=&limit=` searches a cached copy of
[export.iperf3serverlist.net](https://export.iperf3serverlist.net)
(refreshed daily, or on demand via `POST /api/v1/iperf3/servers/refresh`).
Each entry carries `port_end` (the end of the server's advertised port
range, omitted when it only offers a single port), `supports_reverse`,
`supports_udp` and `supports_ipv6` (parsed from the feed's `OPTIONS`
column: `-R`, `-u`, `-6`), plus `gbs`, `continent`, `country`, `site` and
`provider` for display/search.

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

Both `/stats/summary` and `/targets/{id}/history` accept `offset=1` (0 is the
default) to shift the resolved window back by its own span, returning the
immediately preceding period of equal length instead of the current one —
the data behind a "compare with previous period" overlay.

### SLA compliance

Setting a plan speed — General settings `sla_download_mbps`/`sla_upload_mbps`,
or a per-target override via that target's `thresholds.sla_download_mbps`/
`sla_upload_mbps` (either field independently; an unset field falls back to
the general plan) — adds `sla_compliance` to `GET /api/v1/stats/summary`: a
0..1 fraction, per target and overall (the overall figure weighted by each
target's own successful-result count, not a plain average across targets).
It is the share of successful (`status=ok`) results in the window whose
download **and** upload speed both met the resolved plan; `null` when
neither the target nor the general settings set a plan, or per-target when
there were no successful results in the window to judge.

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

- **Paginated endpoints** (`/results`, `/runs`) return
  `{"<plural>": [...], "next_cursor": "..."}`; pass `next_cursor` back as
  `?cursor=` to fetch the next page, and an empty string means the listing
  is exhausted.
- **Collection endpoints** (`/targets`, `/tags`, `/ookla/servers`) return
  `{"<plural>": [...]}`.
- **Single-resource endpoints** return the bare object (e.g.
  `GET /api/v1/targets/{id}` returns the target directly, not wrapped).
- Errors always use the envelope `{"error": {"code": "...", "message": "..."}}`,
  with a matching HTTP status code (400 invalid_request, 404 not_found, 500 internal_error, etc).
- `/results`, `/targets`, `/schedules` and `/runs` send an `ETag`; a matching
  `If-None-Match` gets back `304 Not Modified` with no body.

Identity and auth endpoints:

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/me` | The caller's resolved identity (mode, user, groups, admin) |
| `GET` | `/api/v1/settings/tokens` | List API tokens (metadata only, never the plaintext or hash) |
| `POST` | `/api/v1/settings/tokens` | Create a token; the response is the only time the plaintext is returned |
| `DELETE` | `/api/v1/settings/tokens/{id}` | Revoke a token (refused for the last token while in `token` mode) |

## Observability

- **VictoriaMetrics push** — enable under Settings → Integrations with a URL
  like `http://victoria-metrics:8428`; results are POSTed to
  `/api/v1/import/prometheus` with explicit millisecond timestamps. Series:
  `speedtest_download_bps`, `speedtest_upload_bps`, `speedtest_ping_ms`,
  `speedtest_jitter_ms`, `speedtest_packet_loss_pct`, `speedtest_run_success`,
  `speedtest_duration_ms`; labels `target, target_id, engine, server_id,
  server_name, isp, schedule` plus any extra labels configured in the UI.
  SQLite stays the source of truth: while VM is down up to 200 batches are
  buffered in memory with exponential backoff to 60s, oldest dropped first.
- **VictoriaLogs** — enable with a URL like `http://victoria-logs:9428`; lines
  go to `/insert/jsonline?_stream_fields=app,level&_msg_field=_msg&_time_field=_time`
  in batches of 100 or every 2s. Logs always go to stdout as well; a VL
  outage drops lines rather than blocking the process.
- **`/metrics`** — off by default, toggled by Settings → Integrations →
  *Enable /metrics*, answering 404 while disabled. Exposes
  `speedtest_latest_*` gauges per target, `speedtest_runs_total{status}`,
  `speedtest_runner_queue_depth{lane}`, `speedtest_summary_cache_hits_total` /
  `_misses_total`, and the VM/VL push counters. `/healthz` is always
  available and never gated.
- **`/api/v1/targets/{id}/latest`** — the stable Home Assistant polling
  endpoint, returning the latest result row for a target (404 when it has
  none).
- **Retention** — Settings → General sets results and runs retention in days
  plus the prune interval (default 60 minutes). Pruning deletes in batches of
  1000 per transaction; a run that still owns results is never pruned,
  because `results.run_id` cascades.
- **Docker** — `docker compose up -d` for the app alone, `docker compose
  --profile observability up -d` to add VictoriaMetrics (`:8428`),
  VictoriaLogs (`:9428`) and Grafana (`:3000`, VM pre-provisioned as the
  default datasource).

## Notifications

- **Turning it on** — Settings → Notifications → *Enabled*, then add at least
  one channel. Nothing is delivered while the section is off.
- **Thresholds** — global defaults live in Settings; a target overrides any
  subset in the target form, and a blank field inherits. Limits: minimum
  download and upload in Mbps, maximum ping and jitter in ms, maximum packet
  loss in %, plus *Notify on failed test* for results that never completed.
  A failed result fires only the failure alert — its metric values are zero
  and would otherwise trip everything at once.
- **Firing, cooldown and recovery** — state is tracked per (target, metric)
  in `notification_state`. A breach fires once; while it stays breached it
  re-fires no more often than the cooldown (default 60 minutes). When the
  metric comes back the alert clears and a recovery notification is sent
  unless *Send recovery notifications* is off.
- **Quiet hours** — evaluated in the Settings → General timezone, wrapping
  midnight (22:00-07:00 is a valid window). Deliveries inside the window are
  suppressed and counted in `speedtest_notifications_suppressed_total`; the
  firing state is still recorded, so the morning does not open with a flood
  of overnight alerts.
- **Channels** — `webhook` POSTs the message as JSON (fields: `kind, title,
  body, target, target_id, metric, value, limit, unit, result_id, at`) with
  any configured headers; `ntfy` POSTs the body as text to the full topic URL
  with `Title`, `Priority` and `Tags` headers and an optional bearer token;
  `apprise` POSTs `{title, body, type, tag, urls}` to an Apprise API
  `/notify` endpoint. Each channel has a **Test** button, which uses the
  *saved* channel — save before testing.
- **Reliability** — evaluation and delivery run on the notifier's own
  goroutine behind a 256-slot queue, so a slow or dead channel never delays a
  test; overflow drops the oldest queued result and increments
  `speedtest_notifications_dropped_total`. One failing channel does not stop
  the others.
- **Metrics** — `speedtest_notifications_sent_total`, `_failed_total`,
  `_suppressed_total`, `_dropped_total`, `speedtest_notifications_queued` on
  `/metrics`.

## Docker

```bash
docker run -d --name speedtest-tracker \
  -p 8080:8080 -v speedtest-data:/data \
  ghcr.io/metril/speedtest-tracker:latest
```

Or with `compose.yaml`:

```bash
docker compose up -d                          # app only, on :8080
docker compose --profile observability up -d  # + VictoriaMetrics, VictoriaLogs, Grafana
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
