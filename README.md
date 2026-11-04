# apm_master

Go backend for the APM platform. It ingests OTLP traces, logs and metrics
through Kafka into TimescaleDB and serves the query, alerting, notification,
auth and AI-investigation APIs. It is the source of truth for all platform
data.

Full documentation is in [`docs/`](docs/README.md): architecture, design
and contracts, and operations and configuration.

## Quick start

```bash
make deps-up            # Postgres/TimescaleDB, Redis, Kafka
cp .env.example .env
make run                # API on :8080
make help               # build, lint, test, test-int, migrate-up, …
```

## Layout

```text
cmd/{apm,scheduler,migrate}   entrypoints
internal/app                  dependency wiring (the only place it happens)
internal/platform/*           shared infrastructure; never imports a domain package
internal/<module>             one package per domain: auth ingest traces logs metrics alert investigation notification retention
migrations/                   golang-migrate SQL
```

Before opening a PR, run `make lint && make test` (and `make test-int` if you
touched SQL).
