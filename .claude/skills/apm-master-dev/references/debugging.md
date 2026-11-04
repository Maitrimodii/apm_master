# Debugging — apm_master

Work from evidence: reproduce → locate the layer → form one hypothesis → confirm with a log, query or test → fix → add a regression test. Don't change code until you can explain the failure.

## First moves

1. **Reproduce** with curl and keep the `X-Request-ID`:
   ```bash
   curl -si localhost:8080/api/v1/traces -H "Authorization: Bearer $TOKEN" -H "X-Request-ID: dbg-1"
   ```
   The envelope's `error.code` tells you what happened: `SYS003` validation (see `error.data.field`), `AUTHxxx` auth/permission, `SYS004`/`INV001`… not found, `SYS005` rate limited, `SYS006`/`INV002` illegal state transition (409), `SYS002` Postgres error, `SYS001` unexpected internal error.
2. **Find the log line**: `LOG_LEVEL=debug go run ./cmd/apm` then filter by `request_id`. The logger middleware records `error_type`/`error_code`; the service logs the wrapped cause.
3. **Health**: `/healthz` (process) and `/readyz` (Postgres + Redis).

## Symptom → where to look

| Symptom | Check |
|---|---|
| 401 on every call | `JWT_SECRET` differs between token issuer and server; token expired (`ACCESS_TOKEN_EXPIRY_MINS`); header not `Bearer <jwt>` |
| 403 | role lacks `<resource>:<action>`; check `RequirePermission` args and seeded permissions in `000001_auth.up.sql` |
| 429 `SYS005` | rate limiter: `RATE_LIMIT_BURST` per `RATE_LIMIT_WINDOW_MS`, per client IP (all routes) and per org (after auth). If Redis is down the limiter logs a warning and lets requests through |
| 413 / truncated ingest | `MAX_BODY_BYTES` |
| Ingest returns 2xx but data never appears | Kafka consumer not running or failing: check consumer logs, DLQ topics `apm.telemetry.*.dlq.v1`, consumer lag (below). Then mapper output in `ingest/otlp` |
| Query returns empty | wrong `org_id` (API key belongs to another org), time range in wrong unit/zone (UTC, RFC3339), retention already dropped the chunk |
| Slow query | `EXPLAIN (ANALYZE, BUFFERS)` the store SQL with real params; missing `org_id`-leading index; range too wide for hypertable chunks |
| Alert never fires | `ALERT_EVALUATION_INTERVAL_MS`; `APP_MODE` (scheduler runs in `all` or `scheduler`); instances stay PENDING for the condition window before FIRING (`alert/evaluation_task.go`); check `alerts.condition_parsed` and the source routing in `alert/condition.go` (`logs…`, `spans…`, else metric); another replica may hold the advisory lock |
| Email not sent | `notification_jobs` state; `notification_events.last_error`; SMTP settings |
| AI investigation never starts | `investigation_outbox.published_at` NULL → outbox publisher logs (the writer must have no topic of its own — use `kafkax.NewWriter`); `KAFKA_AI_INVESTIGATION_TOPIC`; ai-service gets 401 on `/internal/v1` → `AI_SERVICE_TOKEN` mismatch; 409 on status updates → the AI tried an illegal transition |
| Shutdown hangs | goroutine not watching `ctx.Done()`; get a dump with `kill -QUIT <pid>` |

## Useful commands

```bash
# Local deps
docker compose -f deploy/docker/docker-compose.yml up -d postgres redis kafka
psql "postgres://apm_user:apm_password@localhost:5433/apm_db"

# Queues and outbox
SELECT state, count(*) FROM notification_jobs GROUP BY 1;
SELECT id, attempts, last_error FROM notification_jobs WHERE state = 'dead_letter' ORDER BY updated_at DESC LIMIT 20;
SELECT * FROM investigation_outbox ORDER BY created_at DESC LIMIT 20;

# Kafka
docker exec apm-kafka kafka-consumer-groups.sh --bootstrap-server localhost:9092 --describe --group apm-master-ingest-writers
docker exec apm-kafka kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic apm.telemetry.traces.dlq.v1 --from-beginning --max-messages 5

# Migrations (refuses to run on a dirty schema unless -force)
go run ./cmd/migrate -action status

# Races / leaks
go test -race -run TestName ./internal/<pkg>/...
```

Interactive debugging: `dlv debug ./cmd/apm` (or the VS Code Go launch config), breakpoint in the handler, then step into service/store.

## After the fix

- Add a test that fails without the fix.
- If the root cause was a missing log/context field that made this hard to find, add it.
- Note in the report: root cause, fix, test added, anything not verified.
