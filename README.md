# FaultHub

Self-hosted crash-report backend for ACRA (Android) clients, with a web
dashboard for browsing and managing crash reports across multiple apps.

## Features

- `POST /api/v1/crash-report` ingest endpoint matching the ACRA HTTP contract
  (flat JSON, `X-API-Key` auth, duplicate `REPORT_ID` dedupe).
- Reports auto-grouped into issues by `STACK_TRACE_HASH` (line numbers
  stripped by ACRA — same bug = one issue).
- Single-admin dashboard: charts, filters, search, pagination, issue
  resolved/open status, raw report view.
- Per-app API keys (created/rotated from the UI, stored as SHA-256 hashes).
- Per-IP token-bucket rate limiting on ingest and login.
- One static binary, pure-Go SQLite, `scratch` Docker image, no Node/npm.
  Charts use a single vendored Chart.js file
  (`internal/web/static/chart.umd.min.js`, v4.5.1) served same-origin —
  no CDN, no build step. To update it, download the pinned UMD build,
  verify the banner version, and record the new sha256 here.

## Quick start

    cp .env.example .env

Set the admin password. On a machine with Go installed, run:

    go run ./cmd/faulthub set-password .env

On a Docker-only host (no Go), run the same command inside a throwaway
container, mounting your `.env` so the container can edit it:

    docker compose build
    docker compose run --rm --user root -v "$PWD/.env":/env.txt faulthub set-password /env.txt

You'll be prompted for the password twice. The command hashes it with argon2id
and writes the result into `.env` as `FAULTHUB_ADMIN_PASSWORD_HASH`, already
escaped for Compose (a raw hash contains `$`, which Compose would otherwise
treat as a variable reference and mangle). What the pieces do:

- `docker compose build` builds the image, so a one-off command can be run from it.
- `docker compose run ... faulthub` starts a throwaway container from that image (`faulthub` is the service name).
- `--rm` deletes that container when the command finishes.
- `-v "$PWD/.env":/env.txt` mounts your host's `.env` into the container at `/env.txt`, giving the command a file it can write — without this it can't reach `.env` on the host.
- `--user root` is needed because the container runs as an unprivileged user that can't write your host's `.env`.
- `set-password /env.txt` is the subcommand and the file to update.

Then start the stack:

    docker compose up -d --build

For local HTTP testing (no TLS), set `FAULTHUB_COOKIE_SECURE=false` in `.env`
before starting — otherwise the browser won't persist the session cookie
over plain HTTP.

Open `http://<host>:8080`, log in with the admin password, create an app, and
copy the API key (shown once).

To change the admin password later, run `set-password` again (same commands as
above), then `docker compose up -d` to recreate the container.

To regenerate or inspect a raw argon2id hash (e.g. for `docker run`/other
setups that don't read `.env`):

    docker compose exec faulthub /faulthub hash-password
    # or locally: go run ./cmd/faulthub hash-password

`hash-password` prints the unescaped hash; `set-password` is the one that
writes it into `.env` ready for Compose. Compose interpolates `.env` values,
so a raw `$argon2id$...` must be escaped as `$$argon2id$$...` — `set-password`
does that for you.

## ACRA client configuration

In the Android app's `local.properties`:

    crashReportEndpoint=https://crashes.example.com/api/v1/crash-report
    crashApiKey=fh_...

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `FAULTHUB_PORT` | `8080` | Port to listen on and publish (docker compose only; sets listen addr and host mapping together) |
| `FAULTHUB_ADDR` | `:8080` | Listen address (running the binary directly; overridden by `FAULTHUB_PORT` under compose) |
| `FAULTHUB_DATA_DIR` | `/data` | SQLite directory |
| `FAULTHUB_ADMIN_PASSWORD` | — | Plaintext admin password (alternative to hash) |
| `FAULTHUB_ADMIN_PASSWORD_HASH` | — | argon2id hash (preferred) |
| `FAULTHUB_INGEST_RATE_PER_MIN` | `30` | Per-IP ingest rate limit |
| `FAULTHUB_INGEST_BURST` | `10` | Ingest burst bucket |
| `FAULTHUB_LOGIN_RATE_PER_MIN` | `5` | Per-IP login rate limit |
| `FAULTHUB_MAX_BODY_BYTES` | `1048576` | Ingest body cap (1 MiB) |
| `FAULTHUB_TRUST_PROXY` | `true` | Use `X-Real-IP`/`X-Forwarded-For` |
| `FAULTHUB_COOKIE_SECURE` | `true` | `Secure` flag on session cookies |
| `FAULTHUB_SESSION_TTL` | `168h` | Session lifetime (sliding) |

Exactly one of `FAULTHUB_ADMIN_PASSWORD` / `FAULTHUB_ADMIN_PASSWORD_HASH`
must be set.

## Reverse proxy

Terminate TLS in front of FaultHub (Caddy/nginx/whatever you use). The proxy
MUST set `X-Real-IP` (or `X-Forwarded-For`) correctly for per-IP rate
limiting to see real client IPs; otherwise all traffic counts as one IP.

## Privacy

`USER_EMAIL` is voluntary PII from the crash dialog: it is never logged,
shown only in the admin report view, and deleted together with its report
(or cascaded via issue/app deletion).

## Backups

The database is a single SQLite file (WAL mode). Do not copy it bare
while the server runs — take a consistent snapshot instead:

    docker compose exec faulthub /faulthub backup /data/backup.db
    docker cp $(docker compose ps -q faulthub):/data/backup.db ./faulthub-backup.db

Or on a schedule from the host via cron, writing into the volume:

    0 3 * * * docker exec faulthub_faulthub-1 /faulthub backup /data/nightly.db

Keep a few rotated copies off the VPS. Restoring is
`docker compose down`, replacing `/data/faulthub.db` with the backup,
and starting again.

## Development

    go test ./...
    go vet ./...
    go run ./cmd/faulthub

Populate a scratch database with demo apps and reports:

    FAULTHUB_ADMIN_PASSWORD=x FAULTHUB_DATA_DIR=/tmp/fh-demo \
      go run ./cmd/faulthub seed