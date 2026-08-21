# Distributed Order Processing API

A Go backend for order processing built around one core idea: **correctness under concurrency**. Before adding queues, gRPC, or event-driven architecture, this project proves the fundamentals hold — no overselling stock, no double-charging on payment retries, no duplicate orders on client retries.

## Highlights

- **Race-safe stock deduction** — row-level locking (`SELECT ... FOR UPDATE`) combined with an atomic conditional update (`UPDATE products SET stock = stock - $1 WHERE stock >= $1`) guarantees no overselling, even under heavy concurrent load. Proven by an integration test that fires 100 concurrent orders against a stock of 10.
- **Deadlock-safe multi-row locking** — when an order touches multiple products, rows are locked in a deterministic order (sorted by product UUID bytes) so concurrent transactions can never deadlock on each other.
- **Idempotency-Key middleware** — clients can safely retry `POST /api/orders/` after a timeout. Requests are deduplicated by key + SHA-256 body hash, in-flight requests are protected from being replayed twice, and stale in-flight keys (>30s) can be reclaimed instead of blocking forever.
- **Idempotent payment flow** — a fake payment provider simulates real-world flakiness (80% success rate). Retrying a payment never double-charges: an already-paid order or already-successful payment short-circuits and returns the existing result.
- **Price snapshotting** — `order_items` stores the price at time of purchase instead of joining live product prices, so historical orders remain accurate even after a product's price changes.
- **Money as integers** — prices are integer minor units end to end (`BIGINT` in every table, `int` in Go). No float ever touches a monetary value, so no amount can be silently rounded on its way to a total.

## Architecture

```text
                    ┌──────────────┐
                    │    Client    │
                    └──────┬───────┘
                           │
                          REST
                           │
                           ▼
                ┌─────────────────────┐
                │       Go API        │
                │                     │
                │ Auth                │
                │ Products            │
                │ Orders              │
                │ Payments            │
                └──────────┬──────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │  Postgres   │
                    └─────────────┘
```

Deliberately a modular monolith for now — no Kafka, no gRPC. The goal was to nail transactions, concurrency, authentication, idempotency, and testing first. The [roadmap](#roadmap) below covers what comes after.

Redis is wired into the app (client setup, `/readyz` health check, exercised in integration tests) but not yet used for any business logic — idempotency keys and everything else currently live in PostgreSQL. It's there as a foundation for a future caching/rate-limiting layer, not a component the current feature set depends on.

## Tech Stack

- **Language**: Go 1.26
- **HTTP router**: [chi](https://github.com/go-chi/chi)
- **Database**: PostgreSQL via [pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev/) (type-safe generated queries), also backs the idempotency-key store
- **Redis**: wired up (client, health check) as groundwork for a future caching layer — not yet used by any business logic
- **Auth**: JWT ([golang-jwt](https://github.com/golang-jwt/jwt)) with explicit algorithm-confusion protection, bcrypt password hashing
- **Validation**: [go-playground/validator](https://github.com/go-playground/validator)
- **Migrations**: [golang-migrate](https://github.com/golang-migrate/migrate)
- **Observability**: [OpenTelemetry](https://opentelemetry.io/) tracing exported to [Jaeger](https://www.jaegertracing.io/), [Prometheus](https://prometheus.io/) metrics, [Grafana](https://grafana.com/) dashboard
- **Testing**: [testify](https://github.com/stretchr/testify), [Testcontainers](https://testcontainers.com/) (real Postgres + Redis in integration tests)
- **CI**: GitHub Actions
- **Containerization**: Docker (multi-stage build, non-root runtime user)

## API Endpoints

Business endpoints are served under the `/api` prefix. Health probes sit at the root so load balancers and orchestrators don't need to know the prefix.

Endpoints marked ✅ expect an `Authorization: Bearer <jwt>` header. A missing, malformed, or expired token returns `401 Unauthorized` with a `WWW-Authenticate: Bearer` header, so clients can treat it as a signal to re-authenticate and retry.

| Method | Path                        | Auth | Notes                                   |
|--------|-----------------------------|------|-----------------------------------------|
| GET    | `/livez`                    | –    | Liveness probe                          |
| GET    | `/readyz`                   | –    | Readiness probe; Redis is non-fatal     |
| POST   | `/api/users/register`       | –    |                                         |
| POST   | `/api/users/login`          | –    | Returns JWT; rate limited               |
| GET    | `/api/users/me`             | ✅   |                                         |
| GET    | `/api/products/`            | –    | Paginated                               |
| POST   | `/api/products/`            | –    |                                         |
| GET    | `/api/products/{id}`        | –    |                                         |
| PATCH  | `/api/products/{id}`        | –    | Partial update                          |
| DELETE | `/api/products/{id}`        | –    |                                         |
| GET    | `/api/orders/`              | ✅   | List current user's orders              |
| GET    | `/api/orders/{id}`          | ✅   |                                         |
| POST   | `/api/orders/`              | ✅   | Requires `Idempotency-Key` header       |
| POST   | `/api/orders/{id}/cancel`   | ✅   | Restores stock                          |
| POST   | `/api/orders/{id}/pay`      | ✅   | Idempotent                              |
| GET    | `/api/orders/{id}/payment`  | ✅   |                                         |

## Concurrency: the core selling point

The scenario that matters: 10 units of stock, 100 users trying to buy 1 unit each, at the same time. Naively reading stock then writing it back is a race condition waiting to happen. Instead, every stock decrement is a single atomic statement:

```sql
UPDATE products
SET stock = stock - $1, updated_at = now()
WHERE id = $2
  AND stock >= $1;
```

If zero rows are affected, the request is rejected with insufficient stock — no read-then-write gap for two requests to race through. This is backed by a `FOR UPDATE` row lock during order placement for defense in depth, and verified end-to-end by `TestOrderConcurrency`:

```text
stock = 10
100 concurrent purchase requests (quantity = 1 each)

Expected:
  10 requests succeed  (201)
  90 requests rejected (409)
  final stock = 0
  exactly 10 order rows created
```

## Observability

Traces, metrics, and logs are wired together rather than bolted on separately.

**Tracing** — every request is traced end to end via OpenTelemetry: HTTP handler, SQL statements (named after the sqlc query, so spans read `CreateOrder` rather than a SQL blob), Redis commands, and explicit spans around the order transaction and its row locks. Traces are exported to Jaeger over OTLP. Business refusals such as "insufficient stock" are recorded as span *attributes*, not span errors, so alerting on span errors keeps meaning something.

**Log correlation** — the slog handler pulls `trace_id` off the active span into every log line, so a log entry leads straight to its trace.

**Metrics** — a Prometheus endpoint exposes request rate, latency histogram, error counts split into 4xx/5xx, in-flight requests, and pgx connection pool statistics. Metrics are labelled by **chi route pattern**, never raw URL: `/api/orders/{id}` is one time series no matter how many order IDs exist, and unmatched paths collapse into a single `unmatched` series so scanner traffic cannot blow up Prometheus' memory.

The metrics endpoint listens on its own port (`METRICS_PORT`, default `9100`) rather than on the API port. Scrapes therefore skip the public router's tracing, auth, and rate limiting, and the endpoint is not exposed alongside the API.

Start the observability stack with the `full` profile and open:

| Service    | URL                       | Notes                                        |
|------------|---------------------------|----------------------------------------------|
| Grafana    | `http://localhost:3001`   | Dashboard "Order API"; anonymous auth, local only |
| Prometheus | `http://localhost:9090`   | Check Status → Targets to confirm scraping   |
| Jaeger     | `http://localhost:16686`  | Trace search                                 |

Grafana's datasource and dashboard are provisioned from `deploy/grafana/`, so they are versioned in the repo instead of living only inside a container. The dashboard covers RPS by route, error rate, latency P50/P95/P99, P99 per route, connection pool usage, and pool saturation.

> Grafana is published on host port 3001 because 3000 collides with most other local dev servers. Override with `GRAFANA_PORT`.

## Getting Started

**Prerequisites**: Go 1.26+, Docker. For the local workflow you also need [golang-migrate](https://github.com/golang-migrate/migrate) on your `PATH`.

There are two ways to run the stack. Pick the local workflow for day-to-day development, and the container workflow for load testing and demos.

### Local development (default)

Postgres and Redis run in containers, the API runs on your machine so you get fast rebuilds.

```bash
git clone <repo-url>
cd distributed-order-processing-api/api

# configure environment (see variables below)
cp .env.example .env
# then fill in POSTGRES_PASSWORD and JWT_SECRET, and update DATABASE_URL to match

# start Postgres + Redis only
docker compose up -d

# run migrations
make migrate-up

# start the API
go run ./cmd/api
```

### Full stack in containers

The `full` Compose profile additionally builds the API image and runs migrations for you:

```bash
docker compose --profile full up -d --build
```

Startup order is enforced by Compose: `db` and `redis` must report healthy, then the one-shot
`migrate` service applies all migrations, and only after it exits successfully does the API start.
No manual `make migrate-up` is needed here.

Inside the Compose network the API talks to `db:5432` and `redis:6379`, so the `DATABASE_URL` and
`REDIS_HOST` in your `.env` (which point at `localhost` for the local workflow) are overridden
automatically — you do not need a second `.env`.

Tear it down with the profile flag as well, otherwise the API container is left running:

```bash
docker compose --profile full down        # add -v to also drop the database volume
```

Verify it came up:

```bash
curl localhost:8080/livez     # process is alive
curl localhost:8080/readyz    # {"success":true,"data":{"db":"up","redis":"up"}}
```

### Environment variables

Required: `DATABASE_URL`, `JWT_SECRET`, `JWT_EXPIRY`, `PORT` (default `8080`), `REDIS_HOST`, `REDIS_PORT`, plus `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` / `POSTGRES_PORT` for Docker Compose.

Observability is configured by `METRICS_PORT` (default `9100`), `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_SAMPLE_RATIO`, and the host ports `PROMETHEUS_PORT` / `GRAFANA_PORT`. Tracing is optional at runtime: if the exporter cannot be reached the app logs a warning and serves traffic without it. `POSTGRES_PORT` and `REDIS_PORT` control the ports published to your host; container-to-container traffic always uses 5432 and 6379.

`.env` is optional at runtime — if the file is absent the app reads configuration straight from the process environment, which is how the containerized profile works.

## Testing

```bash
# unit tests (service layer, mocked repositories)
go test ./...

# integration tests (spins up real Postgres + Redis via Testcontainers, requires Docker)
go test -tags=integration ./test/integration/... -v
```

Both suites run on every push/PR via GitHub Actions.

## Project Structure

```text
api/
├── cmd/api/              # entrypoint
├── internal/
│   ├── user/              # auth, registration, JWT issuance
│   ├── product/            # product catalog
│   ├── order/               # order placement, cancellation, stock logic
│   ├── payment/              # payment processing, fake provider
│   ├── idempotency/           # idempotency key store
│   ├── health/                 # liveness/readiness
│   ├── server/                  # router + app wiring
│   └── platform/                 # config, database, auth, logger, tracing, metrics, middleware
├── deploy/                 # Prometheus config, provisioned Grafana dashboards
├── migrations/              # SQL migrations (golang-migrate)
└── test/integration/         # Testcontainers-based end-to-end tests
```

## Roadmap

This is intentionally a modular monolith today. The plan is to evolve it in stages:

1. **Modular Monolith** *(current)* — single Go service, clean domain boundaries.
2. **gRPC** — split order/inventory concerns into internal services.
3. **Event-driven** — introduce Kafka for order → payment/inventory/notification fan-out.
4. **Observability** *(done)* — OpenTelemetry traces, Prometheus metrics, and trace-correlated structured logs (Prometheus, Grafana, Jaeger).
5. **Load testing** — k6-driven load tests at 1000+ concurrent users, tracking P50/P95/P99 latency and error rate.
