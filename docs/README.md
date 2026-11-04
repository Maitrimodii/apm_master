# APM platform documentation

APM ingests OpenTelemetry traces, logs and metrics, lets operators explore them,
alerts on conditions, and runs AI root-cause investigations on incidents.

| Repository | Stack | Role |
|---|---|---|
| [apm_master](https://github.com/Maitrimodii/apm_master) | Go 1.25, Gin, pgx, TimescaleDB, Kafka, Redis | Ingest, query APIs, alerts, notifications, auth. Source of truth for all data, including investigations. |
| [apm-ai-service](https://github.com/Maitrimodii/apm-ai-service) | Python 3.14, FastAPI, aiokafka, pydantic-ai (Gemini) | Kafka worker that runs one investigation per job and writes the report back to apm_master. |
| [apm-ui](https://github.com/Maitrimodii/apm-ui) | React 18, TypeScript, Redux Toolkit, Vite | Operator dashboard. Talks only to apm_master. |

## Documents

- [architecture.md](architecture.md): components, data flows, who owns which data, package layout rules in each repository, and deployment.
- [design.md](design.md): contracts between services (Kafka job, evidence API, report schema, status machine, error envelope), the alert, notification and investigation lifecycles, and the design decisions behind them.
- [operations.md](operations.md): running locally, configuration, migrations, health checks, retries and dead-letter queues, troubleshooting, and the pre-release checklist.

The HTTP API reference will be generated from OpenAPI specs. Until then the
route list is in [architecture.md](architecture.md#http-surface).

## Five-minute start

Clone the three repositories side by side, then from their parent folder:

```bash
# 1. apm_master and its dependencies
(cd apm_master && make deps-up && make run)        # API on :8080

# 2. AI worker (needs apm_master and Kafka)
(cd apm-ai-service && uv sync && uv run alembic upgrade head && uv run python -m app.worker.main)

# 3. UI
(cd apm-ui && npm install && npm run dev)          # http://localhost:3000
```

Details and environment variables are in [operations.md](operations.md).
