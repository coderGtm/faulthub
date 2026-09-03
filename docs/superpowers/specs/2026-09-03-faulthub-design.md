# FaultHub — Self-Hosted ACRA Crash Reporting Backend

**Date:** 2026-09-03
**Status:** Approved design, ready for implementation planning
**Origin:** Amends the "Crash Reporting Backend API" spec (2026-09-03) produced for Yantra Launcher's ACRA client. That spec's ingest contract is preserved exactly; everything around it is FaultHub's own design.

## 1. Overview

FaultHub is a self-hosted, Dockerized Go service that receives ACRA crash reports
over HTTPS, groups them into issues, and provides a single-admin web dashboard
for browsing, filtering, and managing them across multiple apps.

Decisions made during design (all confirmed by the owner):

| Topic | Decision |
|---|---|
| Users | Single admin (the owner). No public accounts. |
| Apps | Admin creates apps; each app gets its own API key. |
| UI | Server-rendered web dashboard; charts as server-side SVG; hand-written vanilla JS/CSS. **No Node, no JS dependencies** (supply-chain discipline). |
| Database | SQLite, single file on a volume. |
| Rate limiting | Per-IP token buckets, generous + env-configurable, in-app (not just at proxy). |
| TLS | Terminated by the owner's existing reverse proxy; FaultHub serves HTTP. |
| Build | Stdlib-first Go; ~4 small dependencies (see §3). |
| v1 scope | Lean: ingest, issues, dashboard, app/key management, rate limiting, admin auth, delete-with-PII-purge. No retention auto-purge, no webhooks, no export. |

## 2. Ingest Contract (unchanged from ACRA spec — normative)

`POST /api/v1/crash-report`, `Content-Type: application/json`, auth via
`X-API-Key: <key>` header.

- Body is a flat JSON object; keys are ACRA `ReportField` names in
  SCREAMING_SNAKE_CASE; values are strings or `null`.
- Unknown keys are ignored, never rejected. Missing keys are tolerated.
- `REPORT_ID` is required → `400` if absent or longer than 128 chars.
- `STACK_TRACE` is normally required for usefulness but reports without it are
  still accepted (defensive), attributed to a `"(no stack trace)"` issue.
- Success: `201` for a new report, `200` for a duplicate `REPORT_ID`
  (idempotent dedupe — no second row, no 4xx). Response body `{"ok": true}`.
- Missing/wrong API key → `401`, nothing persisted.
- Wrong content type → `415`. Body over the size cap → `413`.
- Rate-limited → `429` with a `Retry-After` header. ACRA treats non-2xx as
  failure and retries on a later app start, so no data is lost.
- Duplicate `REPORT_ID`s are expected (ACRA retries) and handled by primary
  key: `INSERT OR IGNORE`.
- The full original body is stored as `raw` for forward compatibility.
- All report content is untrusted text: HTML-escaped everywhere it is
  rendered; stack traces are rendered as text in `<pre>`, never as HTML.
- `USER_EMAIL` is PII: never logged, shown only in the admin report view, and
  deleted whenever its report or issue is deleted.
- ACRA dates (`EEE MMM d HH:mm:ss zzz yyyy`) are stored as raw text with
  best-effort parsing for display; `received_at` (server UTC) is the
  authoritative timestamp.

Field set (exactly as in the client's `reportContent`): `REPORT_ID`,
`INSTALLATION_ID`, `PACKAGE_NAME`, `APP_VERSION_CODE`, `APP_VERSION_NAME`,
`ANDROID_VERSION`, `BRAND`, `PHONE_MODEL`, `PRODUCT`, `BUILD`, `STACK_TRACE`,
`STACK_TRACE_HASH`, `USER_COMMENT`, `USER_EMAIL`, `USER_APP_START_DATE`,
`USER_CRASH_DATE`, `THREAD_DETAILS`.

## 3. Architecture

One static Go binary; SQLite file under `FAULTHUB_DATA_DIR`; templates and
static assets embedded via `embed`. Listens on plain HTTP (`FAULTHUB_ADDR`,
default `:8080`); TLS is terminated by the owner's reverse proxy.

```
faulthub/
├── cmd/faulthub/main.go      # wiring; subcommands: (serve), hash-password, healthcheck
├── internal/
│   ├── config/               # env parsing with defaults
│   ├── store/                # SQLite open, migrations, all queries
│   ├── ingest/               # POST handler, validation, dedupe, issue upsert
│   ├── ratelimit/            # per-IP token buckets + idle eviction
│   ├── web/                  # admin auth, sessions, CSRF, page handlers
│   ├── charts/               # server-side SVG rendering (line/area, bars)
│   └── keygen/               # API key & session token generation, hashing
├── web/templates/            # embedded html/template files
├── web/static/               # embedded hand-written CSS + vanilla JS
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

Dependencies (the complete list):

| Dependency | Purpose |
|---|---|
| `modernc.org/sqlite` | Pure-Go SQLite driver — no CGO, static builds, `scratch` images |
| `golang.org/x/crypto` | argon2id password hashing |
| `golang.org/x/time/rate` | token-bucket rate limiter |
| (stdlib) | `net/http` routing (Go 1.22+ patterns), `html/template`, `crypto/rand`, `crypto/sha256` |

No router framework, no ORM, no JS/npm anything, no external network calls at
runtime.

## 4. Data Model

```sql
apps (
  id           INTEGER PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  api_key_hash TEXT NOT NULL,           -- SHA-256 hex of the API key
  created_at   TIMESTAMP NOT NULL       -- UTC
)

issues (
  id               INTEGER PRIMARY KEY,
  app_id           INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  stack_trace_hash TEXT NOT NULL,
  title            TEXT NOT NULL,       -- derived: exception class + top app frame
  status           TEXT NOT NULL DEFAULT 'open',   -- 'open' | 'resolved'
  first_seen_at    TIMESTAMP NOT NULL,
  last_seen_at     TIMESTAMP NOT NULL,
  UNIQUE (app_id, stack_trace_hash)
)

reports (
  id                 TEXT PRIMARY KEY,  -- REPORT_ID (ACRA UUID)
  app_id             INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
  issue_id           INTEGER NOT NULL REFERENCES issues(id) ON DELETE CASCADE,
  installation_id    TEXT,
  package_name       TEXT, app_version_code TEXT, app_version_name TEXT,
  android_version    TEXT, brand TEXT, phone_model TEXT, product TEXT,
  build              TEXT, thread_details TEXT,
  stack_trace        TEXT,
  stack_trace_hash   TEXT NOT NULL,
  user_comment       TEXT,
  user_email         TEXT,              -- PII; see §2
  user_app_start_date TEXT, user_crash_date TEXT,  -- raw ACRA text
  received_at        TIMESTAMP NOT NULL,           -- server UTC
  raw                TEXT NOT NULL                 -- original JSON body
)
CREATE INDEX idx_reports_app_time  ON reports(app_id, received_at);
CREATE INDEX idx_reports_issue     ON reports(issue_id);
CREATE INDEX idx_reports_install   ON reports(installation_id);

sessions (
  token_hash TEXT PRIMARY KEY,          -- SHA-256 of session token
  expires_at TIMESTAMP NOT NULL
)
```

Semantics:

- An **app** is the API-key boundary (e.g., "Yantra", "Yantra Pro", other
  projects). `PACKAGE_NAME` flavor differences (`.pro`, `.beta`, `.debug`) are
  per-report data, not separate apps.
- A **report** is one crash event (one `REPORT_ID`).
- An **issue** groups reports with the same `stack_trace_hash` within an app —
  ACRA's hash strips line numbers, so the same bug across devices/versions
  merges into one issue. Navigation works both ways: issue → its reports, and
  report → its issue.
- A new report on a `resolved` issue flips the issue back to `open`.
- Issue `title` is derived on ingest: first line of `STACK_TRACE` (exception
  class and message) plus the first stack frame whose text contains the
  report's `PACKAGE_NAME` (fallback: the first frame at all; if no stack
  trace: `"(no stack trace)"`).
- Deleting a report deletes its `user_email` row data (same row — nothing
  survives). Deleting an issue or app cascades to all child reports, purging
  their PII with them.

## 5. Ingest Pipeline

Order of operations for `POST /api/v1/crash-report`:

1. **Rate limit** (per client IP) → `429` + `Retry-After`.
2. **Content-Type** must be `application/json` (parameters such as `charset`
   tolerated) → `415`.
3. **Auth**: SHA-256 the `X-API-Key`, look up by `api_key_hash` in `apps` →
   `401` if absent/no match. Key format: `fh_` + 43 chars base64url of 32
   random bytes. Only the hash is stored; the plaintext is shown exactly once
   (at creation or rotation).
4. **Body cap** (`FAULTHUB_MAX_BODY_BYTES`, default 1 MiB) → `413`.
5. **Parse** flat JSON: `map[string]any`; string and `null` values kept
   (null → empty), other scalar types stringified defensively, nested
   objects/arrays under known keys ignored; unknown keys ignored entirely;
   malformed JSON → `400`.
6. **Validate** `REPORT_ID` (present, ≤ 128 chars) → `400`.
7. **Store**: `INSERT OR IGNORE` on `reports.id`. If the row exists → `200
   {"ok": true}`. Otherwise: upsert the issue (by `app_id` +
   `stack_trace_hash`; missing hash → sentinel `"(no stack trace)"` group),
   derive/update the issue title, update `first_seen_at`/`last_seen_at`,
   flip `resolved` → `open`, insert the report → `201 {"ok": true}`.

## 6. Rate Limiting

- Token buckets per client IP via `golang.org/x/time/rate`, stored in a map
  guarded by a mutex; buckets idle for > 10 minutes are evicted by a periodic
  sweeper.
- **Ingest endpoint**: `FAULTHUB_INGEST_RATE_PER_MIN` (default 30) with burst
  `FAULTHUB_INGEST_BURST` (default 10). Generous defaults because mobile
  carriers put many users behind one CGNAT IP; a too-tight limit would reject
  legitimate reports from *different* users.
- **Login endpoint**: separate stricter limiter,
  `FAULTHUB_LOGIN_RATE_PER_MIN` (default 5), to blunt brute force.
- Other admin pages are not limited (session-authenticated, single admin).
- Client IP selection: when `FAULTHUB_TRUST_PROXY` (default `true`), use
  `X-Real-IP`, falling back to the last entry of `X-Forwarded-For`; otherwise
  `RemoteAddr` host. Documented requirement: the owner's proxy must set these
  correctly (it does when proxying to a single backend).

## 7. Admin Dashboard

### Authentication

- Single admin. `FAULTHUB_ADMIN_PASSWORD_HASH` (argon2id, encoded; generated
  by `faulthub hash-password`) or `FAULTHUB_ADMIN_PASSWORD` (plaintext, hashed
  at boot — convenience for testing).
- Login form → verify → create session: 32 random bytes, cookie carries the
  raw token, DB stores `sha256(token)` + `expires_at`
  (  `FAULTHUB_SESSION_TTL`, default 7 days, sliding: expiry is extended by the
  full TTL on every authenticated request).
- Cookie: `HttpOnly`, `SameSite=Lax`, `Secure` (toggle:
  `FAULTHUB_COOKIE_SECURE`, default `true`; disable only for local HTTP
  testing).
- CSRF token on every admin POST form, validated server-side.
- Expired sessions are pruned on access and by a periodic sweeper.

### Pages

| Route | Contents |
|---|---|
| `GET /login` | Login form |
| `GET /` | Overview: reports/day chart (last 30d, all apps), per-app report counts, top open issues across apps |
| `GET /apps` | App list; create-app form (name) → API key displayed **once** |
| `POST /apps` | Create app |
| `POST /apps/{id}/rotate-key` | Rotate API key (shows new key once) |
| `POST /apps/{id}/delete` | Delete app + all its data |
| `GET /apps/{id}` | App detail: charts (reports/day; breakdowns by app version, Android version, top devices), key management, stats |
| `GET /apps/{id}/issues` | Issue list: search, filters (status, app version, Android version, device, date range), sort (last seen / count), pagination |
| `GET /apps/{id}/issues/{iid}` | Issue detail: title, status, first/last seen, occurrence count, affected installs, latest stack trace, per-issue charts, occurrence list (paginated, filtered), mark resolved/open, delete issue |
| `POST /apps/{id}/issues/{iid}/status` | Toggle status |
| `POST /apps/{id}/issues/{iid}/delete` | Delete issue + reports |
| `GET /apps/{id}/reports/{rid}` | Report detail: every parsed field + raw JSON; link to its issue; delete report |
| `POST /apps/{id}/reports/{rid}/delete` | Delete single report |
| `GET /healthz` | Liveness (unauthenticated, no data) |

- All pages server-rendered with `html/template` (auto-escaped). Strict CSP:
  `default-src 'self'; style-src 'self'; script-src 'self'`; no external
  assets, fonts, or CDNs. `X-Content-Type-Options: nosniff`,
  `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`.
- Charts: SVG generated server-side in `internal/charts` — area/line for
  over-time series, bars for breakdowns.
- Vanilla JS is limited to progressive niceties (copy-to-clipboard for API
  keys, collapsible rows). Every action works without JS (plain forms).

## 8. Configuration (environment)

| Var | Default | Meaning |
|---|---|---|
| `FAULTHUB_ADDR` | `:8080` | Listen address |
| `FAULTHUB_DATA_DIR` | `/data` | Directory for the SQLite file |
| `FAULTHUB_ADMIN_PASSWORD` | — | Plaintext admin password (convenience) |
| `FAULTHUB_ADMIN_PASSWORD_HASH` | — | argon2id encoded hash (preferred) |
| `FAULTHUB_INGEST_RATE_PER_MIN` | `30` | Per-IP ingest rate |
| `FAULTHUB_INGEST_BURST` | `10` | Ingest burst size |
| `FAULTHUB_LOGIN_RATE_PER_MIN` | `5` | Per-IP login rate |
| `FAULTHUB_MAX_BODY_BYTES` | `1048576` | Ingest body cap |
| `FAULTHUB_TRUST_PROXY` | `true` | Use X-Real-IP / X-Forwarded-For |
| `FAULTHUB_COOKIE_SECURE` | `true` | Set `Secure` on session cookie |
| `FAULTHUB_SESSION_TTL` | `168h` | Session lifetime (sliding) |

Exactly one of `FAULTHUB_ADMIN_PASSWORD` / `FAULTHUB_ADMIN_PASSWORD_HASH` must
be set; the server refuses to start otherwise.

## 9. Docker

- **Dockerfile**, multi-stage:
  1. `golang:1.25` — `CGO_ENABLED=0 go build` → static binary.
  2. `scratch` — the binary only (assets embedded). No shell, no packages,
     no CA store needed (no outbound calls). `USER 65534` (non-root).
     `HEALTHCHECK` using `faulthub healthcheck` (built-in GET against
     `/healthz` on the configured addr).
- **docker-compose.yml**: one `faulthub` service, a named volume mounted at
  `/data`, env vars from `.env`, `restart: unless-stopped`, port mapping
  `8080:8080` for the reverse proxy to forward to.

## 10. Security Summary

- argon2id for the admin password; SHA-256 at rest for API keys and session
  tokens (a DB leak exposes neither).
- API keys 256-bit random, `fh_`-prefixed, rotatable, shown once.
- Per-IP rate limits on ingest and login; body size caps; content-type
  enforcement.
- CSRF tokens on all admin forms; strict CSP; all rendering auto-escaped;
  stack traces always shown as text.
- `USER_EMAIL` and API keys never logged; `USER_CRASH_DATE` etc. stored raw.
- Deletion of any report/issue/app cascades to purge PII.

## 11. Testing

- `go test ./...` with `httptest` and per-test temp-dir SQLite databases.
- **Ingest** (table-driven): sample payload → 201; missing/wrong key → 401
  and nothing stored; missing `REPORT_ID` → 400; duplicate `REPORT_ID` → 200
  with exactly one row; unknown extra keys ignored; non-JSON content type →
  415; oversized body → 413; malformed JSON → 400; report without
  `STACK_TRACE` accepted; concurrent identical `REPORT_ID`s → one row.
- **Rate limiter**: unit tests with an injected clock; burst then 429;
  `Retry-After` present; independent IPs get independent buckets.
- **Admin**: login flow (success/failure), CSRF rejection, session expiry,
  app create/rotate/delete, issue grouping + status flip on new report after
  resolve, filters/search/pagination queries, delete cascades purge
  `user_email`.
- **Charts**: golden-file SVG tests for deterministic inputs.
- **Migrations**: fresh DB creates all tables; re-run is a no-op.

## 12. Out of Scope (v1, deliberately)

- Retention auto-purge (configurable deletion of old reports).
- New-issue webhooks/notifications (Discord/Slack/Telegram/email).
- CSV/JSON export of issue reports.
- Multi-user accounts, OAuth, TOTP 2FA.
- Attachment/multipart ingest (ACRA sends one plain JSON body).
- Any read API for the Android clients (ingest-only, as the ACRA spec says).
- HTTPS inside FaultHub (terminated at the owner's proxy).

Any of these can be added later without breaking the schema (e.g., retention
is a scheduled DELETE; webhook is a goroutine on new-issue upsert).

## 13. Acceptance Checklist

- [ ] `POST /api/v1/crash-report` with the ACRA sample payload and a valid
      app key returns `201` and stores one report + one issue.
- [ ] Missing/wrong `X-API-Key` → `401`, nothing persisted.
- [ ] Body without `REPORT_ID` → `400`.
- [ ] Duplicate `REPORT_ID` → `200`, no duplicate row (incl. concurrent).
- [ ] Unknown JSON keys ignored; missing optional keys tolerated.
- [ ] Non-JSON `Content-Type` → `415`; oversized body → `413`.
- [ ] Sustained ingest from one IP beyond the limit → `429` + `Retry-After`.
- [ ] Reports with equal `STACK_TRACE_HASH` appear as one issue with both as
      occurrences; issue links navigate to reports and back.
- [ ] Admin can create an app, see its API key once, rotate and delete it.
- [ ] Wrong admin password rejected; login rate-limited; CSRF enforced.
- [ ] Deleting a report/issue/app leaves no `user_email` data behind.
- [ ] `docker compose up` builds a scratch image and serves `/healthz`.
- [ ] `go test ./...` and `go vet ./...` pass.
