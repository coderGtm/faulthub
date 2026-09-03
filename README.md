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

## Quick start

    docker compose up -d --build

Then open `http://<host>:8080` behind your TLS reverse proxy, log in with
the admin password, create an app, and copy the API key (shown once).

Generate the admin password hash:

    docker compose exec faulthub /faulthub hash-password
    # or locally: go run ./cmd/faulthub hash-password

## ACRA client configuration

In the Android app's `local.properties`:

    crashReportEndpoint=https://crashes.example.com/api/v1/crash-report
    crashApiKey=fh_...

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `FAULTHUB_ADDR` | `:8080` | Listen address |
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

## Development

    go test ./...
    go vet ./...
    go run ./cmd/faulthub