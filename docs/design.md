# Design

Contracts between the services, the lifecycles they implement, and the
decisions behind the current structure.

## Contracts

### Error envelope

Every apm_master response, including `/internal/v1`, uses one envelope:

```json
{ "success": true,  "data": [ ], "pagination": { "page": 1, "limit": 50, "total": 120 }, "meta": { "request_id": "…", "timestamp": "…" } }
{ "success": false, "error": { "code": "SYS003", "message": "invalid limit", "data": { "field": "limit" } } }
```

| Code | HTTP | Meaning |
|---|---|---|
| `SYS001` | 500 | unexpected internal error (details are logged, never returned) |
| `SYS002` | 500 | database error |
| `SYS003` | 400 | validation error; `error.data.field` names the input |
| `SYS004` | 404 | not found |
| `SYS005` | 429 | rate limited |
| `SYS006` | 409 | conflict / illegal state transition |
| `AUTHxxx` | 401/403 | authentication or permission failure |
| `INV001`–`INV008` | varies | investigation errors (`INV001` not found, `INV002` illegal transition, …) |

The AI service unwraps `data` and maps failures by status: 401 or 403 raises
`BackendAuthError`, 404 raises `NotFoundError`, 409 raises
`InvalidTransitionError` and 5xx raises `BackendUnavailableError`.
Connection errors and 5xx responses are retried with backoff first, up to
`BACKEND_MAX_ATTEMPTS`.

### Histogram buckets

OTLP histograms are stored as one row per bucket with its upper bound `le`
and a per-bucket (not cumulative) count. The overflow bucket's bound is
+Inf, which JSON cannot represent, so both the Kafka wire format and
`GET /metrics/:name/histogram` encode it as `"le": null`.

### Investigation job (Kafka `ai.investigation.jobs`)

```json
{
  "event_id": "uuid",
  "schema_version": "1.0",
  "event_type": "investigation.requested",
  "correlation_id": "uuid",
  "investigation_id": "uuid",
  "incident_id": "uuid",
  "tenant_id": "uuid",
  "created_at": "RFC3339"
}
```

Kafka headers:

| Header | Set by | Purpose |
|---|---|---|
| `correlation_id` | apm_master outbox | propagated to logs and to every internal API call (`X-Correlation-ID`) |
| `x-retry-count` | AI worker | redeliveries used so far |
| `x-not-before` | AI worker | Unix time before which a retried job must not run |
| `x-dlq-reason` | AI worker | `schema_validation_error` or `max_retries_exhausted` |

The worker parses the payload in strict mode. A payload that does not match
goes straight to the DLQ.

### Internal evidence API

`POST /internal/v1/investigation-evidence/query`

```json
{
  "investigation_id": "uuid", "tenant_id": "uuid",
  "query_type": "metric_baseline | slow_traces | trace_details | correlated_logs",
  "time_range_start": "RFC3339", "time_range_end": "RFC3339",
  "parameters": { }
}
```

apm_master runs the query for the tenant and persists the result as evidence.
It returns `{evidence_id, query_type, result, provenance, data_quality_warnings}`.
If the evidence cannot be stored, the request fails, so an `evidence_id` always
refers to a stored row. Each query type has its own executor in
`investigation/evidence`.

`GET /internal/v1/incidents/:id/context` returns the incident, the alert and
`parsed_condition`, the condition as apm_master parsed it. The AI service
reads the alert's metric from `parsed_condition` instead of parsing the
condition string itself.

### Report schema (version 1.0)

```json
{
  "schema_version": "1.0",
  "summary": "string",
  "root_cause": { "hypothesis": "string", "status": "<hypothesis status>", "explanation": "string", "evidence_ids": ["uuid"] },
  "impact": { "affected_services": ["string"], "evidence_ids": ["uuid"] },
  "alternative_hypotheses": [ { "hypothesis": "…", "status": "…", "explanation": "…", "evidence_ids": [] } ],
  "missing_evidence": ["string"],
  "recommended_actions": [ { "action": "string", "reason": "string", "evidence_ids": ["uuid"], "risk": "low|medium|high" } ]
}
```

Hypothesis statuses:

- `supported`: telemetry substantiates the mechanism.
- `confirmed`: verified beyond the telemetry.
- `suspected`: a plausible contributing factor.
- `contradicted`: telemetry disproves it.
- `inconclusive`: an anomaly is visible but the evidence is insufficient.
- `unknown`: telemetry is missing.

Validation rules:

- Every cited `evidence_id` was collected in this investigation.
- A `supported` root cause cites at least one.
- Explanations are not empty.
- No recommended action contains a destructive command (`core/guardrails`).
- Every recommended action cites evidence.

`validation_status` is `valid` or `inconclusive`.

## Lifecycles

### Investigation status

```text
queued ──► running ──► completed | inconclusive | failed | timed_out | cancelled
   └──────────────────► cancelled
```

Both services enforce the same machine (`investigation/enums.go`,
`app/domain/state_machine.py`). apm_master returns 409 for any other
transition. Setting the status an investigation already has is a no-op. The
AI worker reports `running` when it claims a job and the final status after it
saves the report.

| Outcome in the worker | Final status | `failure_code` | Retried |
|---|---|---|---|
| valid report, supported root cause | `completed` | none | no |
| anything else that produced a report | `inconclusive` | none | no |
| run exceeded `MAX_INVESTIGATION_DURATION_SECONDS` | `timed_out` | `INV005` | no |
| tool budget exhausted outside the playbook | `inconclusive` | `TOOL004` | no |
| incident belongs to another tenant | `failed` | `SEC001` | no |
| apm_master rejected the service token | `failed` | `BACKEND002` | no |
| apm_master unreachable or 5xx | `failed` (final attempt only) | `BACKEND001` | yes |
| anything unexpected | `failed` | `SYS001` | no |
| apm_master already cancelled it | local `cancelled`, nothing written | none | no |

The table is `runner.FAILURES`; to add a case, add a row there.

### Alert instance

```text
PENDING ──(breach lasts the window)──► FIRING ──► ACKNOWLEDGED ──► RESOLVED
   └──────────(condition recovers)────────────────────────────────► RESOLVED
```

An alert definition can produce many instances over time. Resolved instances
are never reopened; a new breach creates a new instance.

### Notification job

`pending → processing → sent`, or back to `pending` with a later `run_at`
after a failure, or `dead_letter` once `max_attempts` is exhausted.
High-priority alert jobs carry a `reference_id` so the same notification is
never enqueued twice.

## Reliability design

- **Outbox.** An investigation and its Kafka job are written in one
  transaction. The publisher sends unpublished rows and marks them published,
  so a job is never lost and may be delivered more than once.
- **Idempotent consumers.** The AI worker's claim (`INSERT … ON CONFLICT DO
  NOTHING` plus a lease check under `FOR UPDATE`) makes a redelivery harmless.
  A duplicate delivery is skipped. A delivery whose lease has expired takes the
  job over, which is how a crashed worker's job is recovered.
- **Leases.** Each lease lasts `LEASE_SECONDS` and a heartbeat renews it every
  `LEASE_HEARTBEAT_SECONDS`. When a job will be retried, or the worker is
  shutting down, the worker releases the lease so the next delivery can
  reclaim it at once.
- **Retries.** The worker re-queues a job to `ai.investigation.retry` with an
  exponential `x-not-before` delay of `RETRY_BACKOFF_SECONDS` × 2^(n−1), capped
  at `RETRY_BACKOFF_MAX_SECONDS`. After `MAX_PROVIDER_RETRIES` redeliveries the
  next failure goes to the DLQ. On the final attempt the worker records the
  investigation as `failed` in apm_master, so the job does not stay `running`.
  A tenant at its concurrency limit is re-queued without using up a retry.
- **Batches.** The consumer fetches up to `WORKER_CONCURRENCY` records, runs
  them concurrently and commits offsets once the whole batch is handled.
  `max_poll_interval_ms` is sized to the longest run plus the longest backoff.
- **Budget.** Step 0 is the deterministic playbook, which is limited only by
  `MAX_TOOL_CALLS_TOTAL`. Each model turn of the agent is a further step:
  there are at most `MAX_INVESTIGATION_STEPS` of them, each limited to
  `MAX_TOOL_CALLS_PER_STEP` calls. A repeated identical query is served from
  the cache and does not count against the budget.
- **Untrusted telemetry.** Log lines, span attributes and error messages are
  sanitized (secrets, PII) and wrapped in delimiters in the prompt. The prompt
  tells the model to treat them as data. They never select tools or tenants:
  the tenant and the time window always come from the claimed job.

## Decisions

These come from the structure review. Each one lists the alternative it was
chosen over.

| # | Decision | Over | Why |
|---|---|---|---|
| D1 | apm_master is the source of truth for investigation data; the AI keeps only claims and leases | dual writes with best-effort sync | the dual writes drifted, failures were swallowed, and the UI read only one copy |
| D2 | one flat package per domain module | `handler/ service/ store/` sub-packages | fewer import cycles, unexported helpers stay private, files are named by role |
| D3 | services return domain errors; transport maps them to HTTP | services returning HTTP status codes | services stay transport-free and testable; one mapping table per module |
| D4 | shared infrastructure in `internal/platform/*` | a mixed `internal/pkg` / `common` | one rule: platform never imports a domain package |
| D5 | config with `caarlos0/env` struct tags plus per-section validation | hand-written `os.Getenv` parsing | defaults, types and docs in one place; startup fails on a bad value |
| D6 | alert conditions parsed once at create time and stored as `condition_parsed` JSONB | parsing on every evaluation in both services | invalid rules rejected up front; the AI reads the same parse |
| D7 | OpenAPI-first contracts (planned) | hand-maintained TS types and pydantic models | generated clients stop contract drift; this replaces a hand-written api.md |
| D8 | playbook registry (`Playbook` subclasses) sharing one `EvidenceCollector` with the agent | per-category branches in the orchestrator | a category is one class; the agent and playbook share cache, budget and evidence tracking |
| D9 | package names `traces`, `logs`, `metrics` | `trace`, `log`, `metric` | do not shadow the standard library `log` or common variable names |
| D10 | RTK Query for server state in the UI | hand-written thunks | caching, polling and invalidation built in |
| D11 | CSS Modules | one global stylesheet | styles scoped to components, dead CSS easy to find |
| D12 | MSW for dev-only mocks | mocks inside the API client | production bundle ships no mock data; the same handlers serve tests |
| D13 | short receiver names (`s`, `h`) | long receivers | Go convention; linted |

Tradeoffs we kept:

- **Exact aggregates for alerts.** Metric evaluators use the metrics
  aggregation query (`AVG`, `percentile_cont`) over raw points instead of using continuous aggregates. A
  percentile of pre-aggregated averages would be wrong. Queries over short
  windows (5–15 minutes) stay fast; very wide windows cost a sort.
- **Postgres as the notification queue.** It survives crashes and needs no
  extra infrastructure. It costs polling and lock overhead; `SKIP LOCKED`
  keeps contention low.
- **A deterministic fallback report.** An investigation always ends with a
  report, even without a model. The fallback is worded conservatively: its root
  cause is `supported` only when evidence exists.
- **Unfinished delivery channels.** The Slack and Teams providers are
  placeholders; only SMTP delivers.
