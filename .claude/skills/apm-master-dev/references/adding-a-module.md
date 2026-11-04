# Adding an endpoint or domain module — apm_master

Use `internal/traces` as the reference implementation; it is the smallest complete module.

## New endpoint in an existing module

1. **Types** — request/response structs in `dto.go`, domain rows in `model.go`, filters as one struct (e.g. `ListFilter`) used by both service and store. JSON tags in `snake_case`.
2. **Store** — add the query to `store.go` and its signature to the module's reader interface in `interfaces.go`. Filter by `org_id`. Build optional `WHERE` clauses with the `add(cond, arg)` closure pattern (see `logs/store.go`). Wrap errors; `defer rows.Close()`; return `rows.Err()`.
3. **Service** — business rules; return `apperr.NotFound(...)`, `apperr.Conflict(...)`, `apperr.ValidationError(field, reason)` or `apperr.Internal(err)`; log failures with `ErrorContext`.
4. **Handler** — register the route, then:
   ```go
   func (h *Handler) ListWidgets(c *gin.Context) {
       orgID, apiErr := httpx.OrgID(c)
       if apiErr != nil {
           httpx.Handle(c, apiErr)
           return
       }
       page, apiErr := httpx.ParsePage(c, httpx.DefaultPageSize)
       if apiErr != nil {
           httpx.Handle(c, apiErr)
           return
       }
       items, total, err := h.service.ListWidgets(c.Request.Context(), WidgetFilter{
           OrgID: orgID, Limit: page.Size, Offset: page.Offset(),
       })
       if err != nil {
           httpx.Handle(c, err)
           return
       }
       httpx.List(c, items, total, page.Number, page.Size)
   }
   ```
   Keep non-trivial parsing in a `parseXxxFilter(c) (Filter, httpx.Page, *apperr.Error)` function so it is unit-testable with `gin.CreateTestContext`.
5. **Tests** — parser (valid + each invalid input → error), route registration, service rules with a fake store.

## New domain module

1. Create `internal/<name>/` with `model.go`, `dto.go`, `interfaces.go`, `store.go`, `service.go`, `handler.go`, `module.go`.
2. `module.go`:
   ```go
   func NewModule(deps module.Deps) server.RouteRegistrar {
       service := NewService(NewStore(deps.DB), deps.Logger)
       return NewHandler(service, deps.PermissionChecker, deps.TokenParser, deps.RateLimit)
   }
   ```
   Extra collaborators are explicit parameters typed as interfaces (see `alert.NewModule(deps, notifier)`).
3. Routes: `g := rg.Group("/widgets", middleware.JWTAuth(h.tokenParser))`, then `g.Use(h.rateLimit)` when set, then `middleware.RequirePermission(h.checker, authz.Widgets, authz.Read)` per route.
4. Permissions: add `Widgets Resource = "widgets"` to `internal/authz` and its entries to `authz.Catalogue` (seeded for new orgs). Existing orgs need a migration inserting the permission and granting it to admin roles (see `000010_investigations.up.sql`).
5. Registration is explicit: add a named field and constructor parameter to `server.APIHandler` (or `InternalHandler`) in `internal/server/handler.go`, then pass `widgets.NewModule(deps)` in `internal/app/api.go`.
6. Error codes: module-local `errors.go` with `apperr.Code` constants and, for sentinels, an `apperr.Mapping`.

## Background task

Implement `scheduler.Task` (`Name`, `Interval`, `Execute(ctx)`), return promptly on `ctx` cancellation, and register it in `newScheduler` in `internal/app/infra.go` (used by both binaries). If only one replica may run it, give it a key in `platform/pg/lock.go` and pass `lockFunc(pool, pg.LockX)`.

## Migration

- Next number after the highest in `migrations/` (currently `000012`). Always write `.up.sql` and a `.down.sql` that fully reverses it.
- Tenant tables have `org_id UUID NOT NULL` and an index starting with `org_id`; time-series tables are hypertables with a retention policy.
- Verify: `go run ./cmd/migrate -action up`, `-action down`, `-action up` against a scratch DB; run `make test-int`.

## Config key

```go
// internal/config/config.go, in the right section
WidgetLimit int `env:"WIDGET_LIMIT" envDefault:"100"`
```
Add a rule to that section's `validate` in `validate.go` if a value can be wrong, add the variable to `.env.example`, and add a case to `config_test.go`.
