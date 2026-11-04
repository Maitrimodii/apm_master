# Testing — apm_master

## Test tiers

| Tier | Location | Needs | Run |
|---|---|---|---|
| Unit | `internal/**/_test.go` | nothing | `make test` (`go test ./...`) |
| DB-backed | same files, call a `testPool(t)` helper | `APM_INTEGRATION_DSN` (skips if unset) | `make test-int` (runs with `-count=1 -p 1`; packages share one database) |
| Integration | `test/integration/` | as above | `go test ./test/integration/...` |
| E2E | `test/e2e/**` with `//go:build e2e` | running server, `APM_E2E_URL` | `APM_E2E_URL=http://localhost:8080 go test -tags=e2e ./test/e2e/...` |

`go test ./...` must stay green **offline**: anything needing Postgres/Kafka/Redis must skip when its env var is missing, or carry the `e2e` build tag.

Local deps: `make deps-up` (Postgres on host port 5433). DB tests migrate the database themselves via `pg.Migrate`.

## What to test for each change

- **Handler**: input parsing (valid, each invalid field, boundary values like `limit=0`, `to < from`), route registered with the right method/path, permission middleware present.
- **Service**: every business rule and state transition, including the illegal ones; error mapping (store error → `SYS002`, empty → `SYS004`).
- **Store**: SQL behaviour against a real DB (DB-backed tier), especially `org_id` isolation — insert rows for two orgs and assert the other org's rows never come back. When inserting fixture rows by hand, fill every NOT NULL column (`users` needs `first_name`/`last_name`).
- **Locks and background tasks**: inject the lock as a function (`LockFunc`) so tests can pass a fake that reports "not acquired"; test the real lock in `platform/pg/lock_test.go`.
- **Mappers / pure functions** (`ingest/otlp`, `metric/resolution`, percentiles): table-driven with edge cases (empty arrays, missing attributes, nanosecond timestamps, NaN).
- **Concurrency** (notification worker, consumers, scheduler): run with `-race`; test that `Stop()`/ctx cancel returns.

## Style

```go
func TestParseWidgetParams(testRunner *testing.T) {
    tests := []struct {
        name    string
        target  string
        wantErr bool
        want    int
    }{
        {name: "defaults", target: "/widgets", want: 50},
        {name: "explicit limit", target: "/widgets?limit=10", want: 10},
        {name: "limit too large", target: "/widgets?limit=10000", wantErr: true},
    }

    for _, tc := range tests {
        testRunner.Run(tc.name, func(testRunner *testing.T) {
            params, err := parseWidgetParams(newTestContext(tc.target))
            if tc.wantErr {
                if err == nil {
                    testRunner.Fatal("expected error")
                }
                return
            }
            if err != nil {
                testRunner.Fatalf("parseWidgetParams() error = %v", err)
            }
            if params.Limit != tc.want {
                testRunner.Errorf("Limit = %d, want %d", params.Limit, tc.want)
            }
        })
    }
}
```

- Standard `testing` only (no testify in this repo). Test param is `testRunner` (older files) or `t` — match the file.
- Failure messages: `"Func() field = got, want expected"`.
- `t.Helper()` in helpers, `t.Cleanup` for teardown, `t.Setenv` for env vars, never mutate globals.
- Silence logs with `slog.New(slog.NewJSONHandler(io.Discard, nil))`.
- Gin: `gin.SetMode(gin.TestMode)`; `gin.CreateTestContext(httptest.NewRecorder())` for handlers; `router.Routes()` to assert registration.
- Fakes over mocks: a small struct implementing the consumer-side interface (`traces.Reader`, `alert.Notifier`, `investigation.IncidentReader`) is clearer than a mocking library.
- Use fixed times (`time.Date(...)`) not `time.Now()` when the assertion depends on time.

## Before you say "done"

```bash
make lint         # golangci-lint, includes gofmt/goimports
go vet ./...
make test         # and make test-race when you touched concurrent code
make test-int     # when you touched SQL, migrations or stores
```
Report skipped DB/e2e tiers explicitly.
