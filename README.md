# Web links status service

A simple Go web service.

## Build & Run

Build binary:

```bash
go build -o bin/webserver ./cmd/webserver
```

Run from sources:

```bash
go run ./cmd/webserver
```

By default the service listens on port `8080`.

### Environment variables

| Variable     | Default     | Description                                      |
|--------------|-------------|--------------------------------------------------|
| `PORT`       | `8080`      | HTTP server port.                                |
| `TASKS_FILE` | `tasks.json`| Path to the append-only tasks log on disk.       |
| `MAX_LINKS`  | `50`        | Max number of links accepted in a single request.|
| `MAX_WORKERS`| `100`       | Concurrent link checks per `/links` request.     |
| `HTTP_TIMEOUT`| `5s`       | Per-request timeout for outgoing link checks.    |
| `REPORT_WORKERS` | `2`     | Workers building PDF reports in background.      |

These defaults are defined in `internal/config.Config`. Override them via environment or adjust parsing in `cmd/webserver/main.go` as needed.

## API

### POST /links

Request body:

```json
{"links": ["google.com", "malformedlink.gg"]}
```

Response:

```json
{"links": {"google.com": "available", "malformedlink.gg": "not available"}, "links_num": 1}
```

Each request gets a unique `links_num` persisted in `tasks.json`, so restarts do not lose tasks/results.

### POST /report

Request body:

```json
{"links_list": [1, 2]}
```

Response: PDF report covering all links referenced by those tasks.

Example curl commands:

```bash
# check links
curl -X POST http://localhost:8080/links \
  -H "Content-Type: application/json" \
  -d '{"links": ["google.com", "malformedlink.gg"]}'

# generate a report for saved tasks
curl -X POST http://localhost:8080/report \
  -H "Content-Type: application/json" \
  -d '{"links_list": [1, 2]}' \
  --output report.pdf
```

### GET /metrics

Prometheus endpoint exposing runtime and application metrics.

## Link availability checks

Each link is requested over HTTP (defaults to `https://` if protocol missing). Status values:

- `available` - HTTP 2xx–3xx
- `not available` - request error or any other status

## Restart resilience

- All tasks (`links_num`, links list, results) are serialized to `tasks.json`.
- Writes go via temp file + atomic `rename` to avoid corruption.
- On startup the service restores tasks from `tasks.json`.

## Architecture

Layers:

- `cmd/webserver` - entrypoint: parses config, initializes service, starts HTTP server, manages graceful shutdown.
- `internal/app` - dependency wiring (storage, service, HTTP layer, metrics).
- `internal/domain` - domain models (`Task`, `LinkStatus`) and helper utils.
- `internal/storage` - `FileStorage` append-only log backed by `tasks.json`.
- `internal/service` - business logic: link checking, worker pools, circuit breaker, retries, reporting.
- `internal/httpapi` - HTTP handlers, JSON schemas, context middleware.
- `internal/ports` - shared interfaces (HTTP client, storage, etc.) decoupling layers.
- `internal/pdf` - builds PDF reports from domain tasks.

This structure simplifies testing per layer and swapping infrastructure (e.g. migrating from file storage to DB) without changing the external API.

During restarts in-flight HTTP requests finish gracefully; new ones wait for the next process.

## Architectural patterns

- **Graceful shutdown** - via `signal.NotifyContext` + `http.Server.Shutdown`.
- **Parallel processing** - up to 100 goroutines per `/links` request to handle large batches.
- **Persisted log (append-only)** - each `links_num` append keeps history immutable.
- **Backoff-retry** - exponential retries for transient network errors, respecting context timeouts.

### Key qualities

- **Critical patterns**: circuit breaker, exponential retries, graceful shutdown, worker pools.
- **Security**: SSRF protection (domain validation, private IP deny list), per-IP rate limiting, strict payload validation.
- **Observability**: Prometheus metrics, structured logs, health-check endpoints.
- **Durability**: append-only storage with rotation/cleanup to keep history consistent without bloat.

## Logs

The service relies on Go’s `log/slog` and writes plain text to stdout/stderr. Examples:

- `server listening addr=""` - server start (addr depends on config).
- `load storage: <err>` - failure reading `tasks.json` on startup.
- `server shutdown error: <err>` - graceful shutdown error.

Output is line-oriented plaintext. Use system tooling (systemd journal, docker logs, ELK, etc.) or swap `slog` for structured JSON logging if needed.
