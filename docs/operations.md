# Operations

## Local development

### apm_master

```bash
cd apm_master
make deps-up        # Postgres/TimescaleDB on :5433, Redis, Kafka (deploy/docker/docker-compose.yml)
cp .env.example .env
make run            # go run ./cmd/apm, API on :8080, migrates on start in development
make help           # all targets: build, lint, test, test-race, test-int, migrate-up, migrate-status
```

Smoke test:

```bash
curl -s localhost:8080/healthz
curl -s -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"org_name":"Demo","org_slug":"demo","first_name":"Demo","last_name":"Owner","email":"owner@example.com","password":"StrongPass123!"}'
curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"org_slug":"demo","email":"owner@example.com","password":"StrongPass123!"}'
curl -s -X POST localhost:8080/api/v1/auth/api-keys -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"name":"demo-key"}'
```

### apm-ai-service

```bash
cd apm-ai-service
uv sync
cp .env.example .env              # DATABASE_URL, KAFKA_BROKERS, APM_BACKEND_INTERNAL_URL, token
uv run alembic upgrade head
uv run python -m app.worker.main  # the worker; this is what runs investigations
uv run uvicorn app.main:app --port 8001   # optional: health endpoints only
```

With no real `GEMINI_API_KEY` (any value that starts with `test`), the worker
skips the model and writes deterministic playbook reports. This is useful for
local runs and tests.

`deploy/docker-compose.yml` starts Postgres, Kafka, the API and the worker
together.

### apm-ui

```bash
cd apm-ui
npm install
cp .env.example .env
npm run dev        # http://localhost:3000; proxies /api/v1, /healthz, /readyz to VITE_BACKEND_URL
npm run dev:mock   # same, but MSW serves mock data (no backend needed)
```

Set `VITE_BACKEND_URL` in `.env` to apm_master's address (default
`http://localhost:8080`).

## Configuration

### apm_master

Configuration is read from the environment; a `.env` file is loaded if one
exists. Typed sections live in `internal/config/config.go`. Startup fails on an
invalid value. Outside development it also fails when `AI_SERVICE_TOKEN` is
still the placeholder.

| Variable | Purpose | Default |
|---|---|---|
| `APP_ENV` / `APP_MODE` | environment name / `all`, `api`, `scheduler` | development / all |
| `APP_URL` (legacy `APPURL`) | base URL for links in emails | http://localhost:3000 |
| `CORS_ALLOWED_ORIGINS` | comma-separated browser origins | `APP_URL` |
| `AUTO_MIGRATE` | apply migrations at startup | true in development only |
| `LOG_LEVEL` | log verbosity | info |
| `HTTP_PORT`, `HTTP_*_TIMEOUT_MS`, `MAX_BODY_BYTES` | HTTP server | 8080, 10s/30s/120s/5s, 10 MB |
| `RATE_LIMIT_BURST` / `RATE_LIMIT_WINDOW_MS` | requests per window, per client IP and per org | 200 / 1000 |
| `POSTGRES_HOST/PORT/USER/PASSWORD/DB` | database | localhost/5432/apm_user/—/apm_db |
| `POSTGRES_TIMEOUT_MS`, `POSTGRES_MAX_CONNS`, `POSTGRES_MIN_CONNS`, `POSTGRES_MAX_CONN_*_MS` | pool | |
| `REDIS_URL` / `REDIS_TIMEOUT_MS` | rate-limit store | localhost:6379 / 5s |
| `JWT_SECRET` | token signing key, at least 32 characters | required |
| `ACCESS_TOKEN_EXPIRY_MINS`, `REFRESH_TOKEN_EXPIRY_DAYS`, `INVITE_EXPIRY_DAYS`, `PASSWORD_RESET_EXPIRY_MINS` | auth lifetimes | 15 / 7 / 10 / 30 |
| `EMAIL_FROM`, `SMTP_HOST/PORT/USERNAME/PASSWORD` | email; nothing is sent while `SMTP_HOST` is empty | |
| `KAFKA_BROKERS`, `KAFKA_CLIENT_ID`, `KAFKA_CONSUMER_GROUP` | Kafka | localhost:9092 / apm-master-ingest / apm-master-ingest-writers |
| `KAFKA_{TRACES,LOGS,METRICS}_TOPIC` / `…_DLQ_TOPIC` | telemetry topics | `apm.telemetry.*.v1` / `.dlq.v1` |
| `KAFKA_TOPIC_PARTITIONS/REPLICATION_FACTOR/RETENTION_MS` | topic provisioning | 12 / 1 / 7 days |
| `KAFKA_PRODUCER_*` | compression, batching, acks, attempts | lz4, 1000 records, 1 MB, 10 ms |
| `KAFKA_CONSUMER_*` | workers, batching, retry | 3 workers, 1000 records, 1 s flush, 5 attempts |
| `ALERT_EVALUATION_INTERVAL_MS` | alert evaluation cadence | 60000 |
| `NOTIFICATION_RETENTION_DAYS`, `REFRESH_TOKEN_RETENTION_DAYS` | retention of audit rows | 90 / — |
| `KAFKA_AI_INVESTIGATION_TOPIC`, `KAFKA_AI_RETRY_TOPIC`, `KAFKA_AI_DLQ_TOPIC` | AI job topics | `ai.investigation.jobs` / `.retry` / `.dlq` |
| `AI_SERVICE_TOKEN` | token the AI worker sends to `/internal/v1` | placeholder (development only) |
| `INVESTIGATION_OUTBOX_POLL_INTERVAL_MS` / `…_BATCH_SIZE` | outbox publisher | 2000 / 50 |

### apm-ai-service

All settings are fields of `app/config.py:Settings`, read from the environment
or `.env`.

| Variable | Purpose | Default |
|---|---|---|
| `DATABASE_URL`, `DB_POOL_*` | claim/lease database | postgresql+asyncpg://…/ai_db |
| `KAFKA_BROKERS`, `KAFKA_CONSUMER_GROUP` | Kafka | localhost:9092 / ai-investigation-workers |
| `KAFKA_INVESTIGATION_TOPIC`, `KAFKA_RETRY_TOPIC`, `KAFKA_DLQ_TOPIC` | must match apm_master | `ai.investigation.jobs` / `.retry` / `.dlq` |
| `WORKER_CONCURRENCY` | jobs run at once per worker | 4 |
| `RETRY_BACKOFF_SECONDS` / `RETRY_BACKOFF_MAX_SECONDS` | retry delay base / cap | 5 / 300 |
| `MAX_PROVIDER_RETRIES` | redeliveries before the DLQ | 3 |
| `APM_BACKEND_INTERNAL_URL`, `APM_BACKEND_SERVICE_TOKEN` | apm_master and `AI_SERVICE_TOKEN` | http://localhost:8080 / placeholder |
| `BACKEND_TIMEOUT_SECONDS`, `BACKEND_EVIDENCE_TIMEOUT_SECONDS`, `BACKEND_MAX_ATTEMPTS` | client timeouts and retries | 10 / 15 / 3 |
| `GEMINI_API_KEY`, `GEMINI_MODEL` | model; a key that starts with `test` disables it | test-key / gemini-2.0-flash |
| `MAX_INVESTIGATION_STEPS`, `MAX_TOOL_CALLS_TOTAL`, `MAX_TOOL_CALLS_PER_STEP` | budget | 8 / 16 / 3 |
| `MAX_INVESTIGATION_DURATION_SECONDS`, `MODEL_CALL_TIMEOUT_SECONDS`, `TOOL_CALL_TIMEOUT_SECONDS` | timeouts | 120 / 30 / 15 |
| `MAX_CONCURRENT_INVESTIGATIONS_PER_TENANT` | per-tenant limit | 2 |
| `LEASE_SECONDS`, `LEASE_HEARTBEAT_SECONDS` | job lease | 150 / 30 |
| `INCIDENT_WINDOW_MINUTES` | evidence window either side of the job time | 15 |
| `SLOW_TRACES_LIMIT`, `LOGS_LIMIT`, `METRIC_SAMPLES_LIMIT`, `MAX_TOOL_RESULT_BYTES` | query sizes | 10 / 50 / 300 / 32 KB |

## Migrations

```bash
# apm_master (golang-migrate); refuses to run on a dirty schema unless -force
go run ./cmd/migrate -action up|down|reset|status
go run ./cmd/migrate -action force -version N     # mark N clean after a manual fix
go run ./cmd/migrate -action clear-telemetry      # empty spans, logs and metrics

# apm-ai-service (Alembic)
uv run alembic upgrade head
```

Run migrations before rolling out new code. AI migration `0002` drops the
local `investigation_events`, `investigation_evidence` and
`investigation_reports` tables. That data has always been written to
apm_master as well, so nothing is lost.

## Health and monitoring

| Check | apm_master | apm-ai-service |
|---|---|---|
| Liveness | `GET /healthz` | `GET /healthz` |
| Readiness | `GET /readyz` (Postgres + Redis) | `GET /readyz` (its database) |
| Logs | JSON `slog`, with `request_id`, `correlation_id`, `error_code` | JSON `structlog`, with `correlation_id`, `investigation_id`, Kafka topic/partition/offset |

Watch:

- the lag of consumer group `apm-master-ingest-writers` on `apm.telemetry.*.v1`
- messages arriving on any `*.dlq*` topic
- `investigation_outbox` rows with `published_at IS NULL` older than a minute
- `notification_jobs` in `dead_letter`
- investigations stuck in `running` for longer than `MAX_INVESTIGATION_DURATION_SECONDS` plus the lease

Useful queries and commands:

```sql
SELECT state, count(*) FROM notification_jobs GROUP BY 1;
SELECT id, attempts, last_error FROM notification_jobs WHERE state = 'dead_letter' ORDER BY updated_at DESC LIMIT 20;
SELECT * FROM investigation_outbox WHERE published_at IS NULL ORDER BY created_at;
-- ai_db
SELECT i.id, i.status, i.attempt_number, i.lease_expires_at, a.worker_id, a.outcome
FROM investigations i JOIN investigation_attempts a ON a.investigation_id = i.id
ORDER BY a.started_at DESC LIMIT 20;
```

```bash
docker exec apm-kafka kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --group apm-master-ingest-writers
docker exec apm-kafka kafka-console-consumer.sh --bootstrap-server localhost:9092 \
  --topic ai.investigation.dlq --from-beginning --property print.headers=true --max-messages 5
```

## Retries and dead-letter queues

| Pipeline | Retry | Dead letter |
|---|---|---|
| Telemetry ingest consumers | in-process, `KAFKA_CONSUMER_MAX_ATTEMPTS` with backoff | `apm.telemetry.<signal>.dlq.v1` |
| Notifications | `run_at` pushed out by `2^attempts` seconds, capped at 5 minutes | `notification_jobs.state = 'dead_letter'` |
| AI jobs | `ai.investigation.retry` with an `x-not-before` delay | `ai.investigation.dlq` with `x-dlq-reason` |

To replay an AI job from the DLQ, produce its payload to
`ai.investigation.jobs` without the `x-*` headers. The claim makes the replay
safe. A user can also press **Retry** on the investigation, which calls
`POST /investigations/:id/retry`.

## Troubleshooting

| Symptom | Look at |
|---|---|
| 401 on every request | `JWT_SECRET` differs between issuer and server, the token expired, or the header is not `Bearer <jwt>` |
| 403 | the role lacks `<resource>:<action>`; check the seeded permissions (migration 000011 uses plural resource names) |
| 429 `SYS005` | rate limit per IP or per org. If Redis is down the limiter logs a warning and allows requests. |
| Ingest returns 2xx but data never appears | consumer logs, consumer lag, the telemetry DLQ topics |
| A query returns nothing | wrong org (the API key belongs to another org), time range not in UTC RFC3339, or retention already dropped the data |
| An alert never fires | `APP_MODE` must be `all` or `scheduler`; instances stay `PENDING` for the condition window; check `alerts.condition_parsed`; another replica may hold the advisory lock |
| No emails | `SMTP_HOST` unset; check `notification_jobs` and `notification_events.last_error` |
| An investigation stays `queued` | outbox row unpublished (publisher logs), AI worker not consuming (group lag), or the tenant is at its concurrency limit (worker logs "re-queueing") |
| An investigation stays `running` | worker crashed: another worker takes it over once the lease expires on the next delivery. If the final status update failed, the worker logs "failed to report final status". |
| AI worker logs 401 from `/internal/v1` | `APM_BACKEND_SERVICE_TOKEN` differs from `AI_SERVICE_TOKEN` |
| AI worker logs 409 | apm_master refused the transition, usually because a user cancelled the investigation; the worker skips it |
| Every report is a synthesized playbook report | `GEMINI_API_KEY` starts with `test`, or the model times out (`MODEL_CALL_TIMEOUT_SECONDS`) |

Interactive debugging: `dlv debug ./cmd/apm` for Go; for Python, run the
worker under `debugpy` or call `app.worker.runner.run_investigation` directly
with a hand-built event.

## Before a release

```bash
# apm_master
make lint && go vet ./... && make test && make test-race
APM_INTEGRATION_DSN=… make test-int        # DB-backed tier
# apm-ai-service
uv run ruff check . && uv run ruff format --check . && uv run mypy && uv run pytest
# apm-ui
npm run typecheck && npm run lint && npm run format:check && npm test && npm run build
```

Then:

- Apply migrations and confirm `/readyz` on every service.
- Register a user, create an API key and send sample telemetry.
- Check that `/api/v1/traces`, `/logs` and `/metrics` return the data.
- Create an alert and confirm it fires.
- Start an investigation and confirm it reaches `completed` or `inconclusive` with a report.
