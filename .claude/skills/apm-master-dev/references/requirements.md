# Requirement gathering — apm_master

Use before writing code whenever the request leaves behaviour open. Ask only the questions whose answers would change the implementation; answer the rest yourself from the code and state the assumption.

## 1. Restate the request

Write one paragraph: who calls it (UI user, OTLP sender with API key, apm-ai-service with service token, background task), what they get, and why. If you can't, you don't understand it yet.

## 2. Questions to resolve

**Contract**
- Method, path, and which group: `/api/v1` (JWT + permission), `/api/v1/ingest` (API key), or `/internal/v1` (service token)?
- Request fields: types, required vs optional, defaults, bounds (time range max, `limit` max, string lengths).
- Response shape inside `data`. Is it a list (needs pagination: `page`, `limit`, `total`)?
- Which error cases exist and which status/`Code` each maps to? Is a new `Code` needed?

**Authorization and tenancy**
- Which permission resource and action (`traces`/`logs`/`metrics`/`alerts`/… × read/write)? Does a new resource need seeding in a migration?
- Is data strictly scoped to `org_id`? Any cross-org admin case? (Default: no.)

**Data**
- New table/column? Hypertable or regular table? Retention policy? Indexes needed for the query's `WHERE`/`ORDER BY`?
- Expected volume (rows/sec for ingest, rows scanned per query). This decides COPY vs INSERT, index choice, and whether a time-range limit is mandatory.
- Backfill or migration of existing rows?

**Behaviour and lifecycle**
- State machine changes (alert instance FIRING → ACKNOWLEDGED → RESOLVED; investigation statuses)? Which transitions are illegal?
- Side effects: notifications (priority 10 alert vs 0 transactional), outbox event to Kafka, audit/event rows?
- Idempotency: what happens if the same request/event arrives twice?
- Sync vs async: must the caller wait, or is it a job/queue?

**Operational**
- New config keys and their defaults? Different per environment?
- What should be logged at Info vs Debug? Any metric or health impact?
- Does it run in the API process, the scheduler (`APP_MODE`), or both?

**Consumers**
- Does `apm-ui` or `apm-ai-service` need to change? Who owns that change?

## 3. Write the spec

Produce this before implementing anything non-trivial and confirm it with the user:

```markdown
### <Feature name>
**Goal:** one sentence.
**Endpoint / trigger:** METHOD /path (auth: JWT+perm `<resource>:<action>` | API key | service token)
**Request:** fields with types, required, bounds
**Response (`data`):** shape; pagination yes/no
**Errors:** condition → HTTP status → Code
**Data changes:** migration 0000NN_<name> (tables, columns, indexes, retention)
**Config:** NEW_KEY (default, meaning)
**Out of scope:** …
**Acceptance criteria:**
- [ ] Given … when … then …  (one per behaviour, including tenancy and each error)
**Tests:** which unit/handler/integration tests will prove each criterion
```

Acceptance criteria must be checkable by a test or a curl command. "Fast" or "robust" are not criteria; "p95 < 200 ms for 1 h range over 10 M spans" is.
