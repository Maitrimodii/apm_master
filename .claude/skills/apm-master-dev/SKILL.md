---
name: apm-master-dev
description: Use when working on the apm_master Go backend (Gin + pgx + Kafka + TimescaleDB + Redis) — adding or changing an API endpoint or domain module, editing OTLP ingest, Kafka producer/consumer, traces/logs/metrics queries, alerts and the condition parser, notifications, retention, the investigation outbox and /internal/v1 evidence APIs, migrations, or config; also when writing Go tests, debugging a failing request or pipeline, reviewing Go code, or turning a feature request into a spec for this service.
---

# apm_master development

Go 1.25 service. Module path `github.com/Maitrimodii/apm_master`. `cmd/apm` serves the public API, runs Kafka ingest consumers, the notification worker, the investigation outbox and, when `APP_MODE=all` (default), the scheduler (alert evaluation + retention). `cmd/scheduler` runs the scheduler alone; `cmd/migrate` manages the schema. All wiring lives in `internal/app`.

Pick the reference file for the task at hand:

| Task | Read |
|---|---|
| Clarifying a feature / writing acceptance criteria | [references/requirements.md](references/requirements.md) |
| Adding an endpoint or a whole domain module | [references/adding-a-module.md](references/adding-a-module.md) |
| Writing or fixing tests | [references/testing.md](references/testing.md) |
| Something is broken / slow / missing data | [references/debugging.md](references/debugging.md) |

## Code map

```text
cmd/apm, cmd/scheduler    thin: config.Load → logger → app.RunAPI / app.RunScheduler
cmd/migrate               -action up|down|reset|status|force|clear-telemetry [-dir] [-force]
internal/app/             builds the dependency graph and shutdown sequence (only place that does)
internal/config/          typed sections with `env`/`envDefault` tags; Validate / ValidateScheduler
internal/authz/           Resource + Action constants, Checker interface, default permission Catalogue
internal/server/          HTTP bootstrap, APIHandler/InternalHandler (explicit fields), health probes
internal/platform/        shared infrastructure — must never import a domain package
  apperr/                 Error{Type, Code, Message, Data, Cause}, constructors, Mapping (sentinel → kind)
  httpx/                  envelope (OK/Created/Accepted/List/NoContent), Handle, ParsePage/ParseTimeRange/
                          QueryInt/QueryFloat/PathUUID, OrgID/UserID/OrgAndUser/BearerToken, header + Gin keys
  middleware/             JWTAuth, APIKeyAuth, ServiceTokenAuth(token), RequirePermission, CORS(origins),
                          RateLimiter.ByClientIP()/ByOrg(), RequestID, CorrelationID, Logger, Recovery, BodyLimit
  pg/                     Connect(ctx, dsn, PoolConfig), Migrate*, WithAdvisoryLock + lock keys
  redisx/ kafkax/ logger/ ctxkeys/ retry/ module/(Deps)
internal/<domain>/        traces, logs, metrics, alert, investigation, notification, auth — one package each:
                          model.go dto.go interfaces.go store.go service.go handler*.go module.go (+ constants/enums)
internal/alert/           condition.go (ParseExpression), evaluator*.go, notifier.go
internal/investigation/   handler_public.go, handler_internal.go, evidence_service.go, outbox.go, evidence/ (executors)
internal/ingest/          handler → services → producer (sync) ; consumer → writer (COPY) ; model/wire.go (Kafka V1 types)
migrations/               0000NN_name.up.sql / .down.sql
```

## Layering rules

Request flow is **handler → service → store**.

- **Handler**: parse input with `httpx` helpers (they return 400 on bad input; never `strconv` with `_`), read identity with `httpx.OrgID(c)` / `httpx.OrgAndUser(c)`, call the service, respond with `httpx.OK/Created/Accepted/List/NoContent` or `httpx.Handle(c, err)`.
- **Service**: business rules and state transitions. Returns `*apperr.Error` (`apperr.NotFound`, `apperr.Conflict`, `apperr.ValidationError`, `apperr.Internal(err)`) or module sentinel errors that the handler maps with an `apperr.Mapping` (see `investigation/errors.go`). Never imports `httpx` or Gin.
- **Store**: SQL only, errors wrapped with `fmt.Errorf("verb noun: %w", err)`. **Every tenant query filters by `org_id`.** Return `(nil, nil)` for not-found rows and let the service decide.
- **Module** (`module.go`): `NewModule(deps module.Deps, …) server.RouteRegistrar` builds store → service → handler.
- Services depend on small consumer-side interfaces (`traces.Reader`, `alert.Notifier`, `investigation.IncidentReader`), not concrete stores of other modules.

## Conventions to match

- **Receivers and params are short**: `h *Handler`, `s *Service`, `c *gin.Context`. Tests may use `t` or `testRunner` (match the file).
- **Constants over literals**: permission checks use `authz.<Resource>`/`authz.<Action>`; headers and Gin keys use `httpx.Header*`/`httpx.CtxKey*`; Go context values use `platform/ctxkeys`; notification event types use `notification.Event*`; statuses use the module's typed enum.
- **Routes**: public under `/api/v1` — group with `middleware.JWTAuth(tokenParser)`, then `deps.RateLimit`, then `middleware.RequirePermission(checker, authz.X, authz.Read)` per route. Internal routes under `/internal/v1` use `middleware.CorrelationID()` + `middleware.ServiceTokenAuth(cfg.AI.ServiceToken)`. Ingest uses `APIKeyAuth`. Every response uses the envelope.
- **Errors**: new failure scenario → new `apperr.Code` (module-local block like `investigation/errors.go`, or `platform/apperr` for shared ones). Messages are client-safe.
- **Logging**: `log/slog`, injected logger, `*Context` variants, snake_case keys.
- **Config**: add a tagged field to the right section in `internal/config/config.go`, a rule in `validate.go` if it can be wrong, and the variable in `.env.example`. `*_MS` fields are `time.Duration`.
- **Background work**: goroutines exit on `ctx.Done()` and are stopped in `internal/app`. Cluster singletons use `pg.WithAdvisoryLock` via `lockFunc`.
- **Kafka**: writers come from `kafkax.NewWriter` (no writer-level topic; messages carry it). Kafka payloads use `ingest/model` wire types, not API models.
- **Readability**: guard clauses, functions that fit on a screen, comments that say why.

## Workflow

1. **Understand** — read the module end to end (handler → service → store → migration) and its tests. Vague request → [references/requirements.md](references/requirements.md).
2. **Plan** — files, error codes, migration, config keys, tests.
3. **Implement** in small steps. If a JSON shape under `/api/v1` or `/internal/v1` changes, list the consumer files: `apm-ui/src/api/*` for `/api/v1`, `apm-ai-service/app/clients/apm_backend.py` for `/internal/v1`.
4. **Verify** from `apm_master/`:
   ```bash
   make test          # go test ./...
   make lint          # golangci-lint (gofmt/goimports included)
   go vet ./...
   make test-int      # DB-backed tier, needs Postgres (make deps-up)
   ```
   Add `-race` when you touched goroutines or shared state.
5. **Report** what changed, what you ran and the result, and anything unverified.

## Never

- Return raw errors to clients, log-and-swallow errors, or `_ =` a `strconv`/`json` result.
- Query tenant data without `org_id`, or take `org_id` from the request body instead of the auth context.
- Query another module's tables directly — go through its exported reader.
- Edit an applied migration — add a new numbered pair with a working `.down.sql`.
- Hardcode topics, timeouts, limits, URLs or secrets.
- Run session-level advisory locks through `pool.QueryRow` (the unlock can land on another connection) — use `pg.WithAdvisoryLock`.
- `panic` outside startup, or add a dependency without saying why.
