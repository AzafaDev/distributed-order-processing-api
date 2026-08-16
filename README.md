# Distributed Order Processing API

A Go backend for order processing built around one core idea: **correctness under concurrency**. Before adding queues, gRPC, or event-driven architecture, this project proves the fundamentals hold — no overselling stock, no double-charging on payment retries, no duplicate orders on client retries.

## Highlights

- **Race-safe stock deduction** — row-level locking (`SELECT ... FOR UPDATE`) combined with an atomic conditional update (`UPDATE products SET stock = stock - $1 WHERE stock >= $1`) guarantees no overselling, even under heavy concurrent load. Proven by an integration test that fires 100 concurrent orders against a stock of 10.
- **Deadlock-safe multi-row locking** — when an order touches multiple products, rows are locked in a deterministic order (sorted by product UUID bytes) so concurrent transactions can never deadlock on each other.
- **Idempotency-Key middleware** — clients can safely retry `POST /orders` after a timeout. Requests are deduplicated by key + SHA-256 body hash, in-flight requests are protected from being replayed twice, and stale in-flight keys (>30s) can be reclaimed instead of blocking forever.
- **Idempotent payment flow** — a fake payment provider simulates real-world flakiness (80% success rate). Retrying a payment never double-charges: an already-paid order or already-successful payment short-circuits and returns the existing result.
- **Price snapshotting** — `order_items` stores the price at time of purchase instead of joining live product prices, so historical orders remain accurate even after a product's price changes.

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
                └──────┬──────┬───────┘
                       │      │
                ┌──────▼──┐ ┌─▼──────┐
                │Postgres │ │ Redis  │
                └─────────┘ └────────┘
```

Deliberately a modular monolith for now — no Kafka, no gRPC. The goal was to nail transactions, concurrency, authentication, idempotency, and testing first. The [roadmap](#roadmap) below covers what comes after.

## Tech Stack

- **Language**: Go 1.26
- **HTTP router**: [chi](https://github.com/go-chi/chi)
- **Database**: PostgreSQL via [pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev/) (type-safe generated queries)
- **Cache / dedup store**: Redis
- **Auth**: JWT ([golang-jwt](https://github.com/golang-jwt/jwt)) with explicit algorithm-confusion protection, bcrypt password hashing
- **Validation**: [go-playground/validator](https://github.com/go-playground/validator)
- **Migrations**: [golang-migrate](https://github.com/golang-migrate/migrate)
- **Testing**: [testify](https://github.com/stretchr/testify), [Testcontainers](https://testcontainers.com/) (real Postgres + Redis in integration tests)
- **CI**: GitHub Actions
- **Containerization**: Docker (multi-stage build, non-root runtime user)

## API Endpoints

| Method | Path                      | Auth | Notes                          |
|--------|---------------------------|------|---------------------------------|
| POST   | `/users/register`         | –    |                                  |
| POST   | `/users/login`            | –    | Returns JWT                     |
| GET    | `/users/me`                | ✅   |                                  |
| GET    | `/products`               | –    | Paginated                       |
| POST   | `/products`               | –    |                                  |
| GET    | `/products/{id}`          | –    |                                  |
| PATCH  | `/products/{id}`          | –    | Partial update                  |
| DELETE | `/products/{id}`          | –    |                                  |
| GET    | `/orders/`                 | ✅   | List current user's orders      |
| GET    | `/orders/{id}`             | ✅   |                                  |
| POST   | `/orders/`                 | ✅   | Requires `Idempotency-Key` header |
| POST   | `/orders/{id}/cancel`      | ✅   | Restores stock                  |
| POST   | `/orders/{id}/pay`         | ✅   | Idempotent                      |
| GET    | `/orders/{id}/payment`     | ✅   |                                  |

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

## Getting Started

**Prerequisites**: Go 1.26+, Docker.

```bash
git clone <repo-url>
cd distributed-order-processing-api/api

# configure environment (see variables below)
cp .env.example .env   # create your own if not present

# start Postgres + Redis
docker compose up -d

# run migrations
make migrate-up

# start the API
go run ./cmd/api
```

Required environment variables: `DATABASE_URL`, `JWT_SECRET`, `JWT_EXPIRY`, `PORT` (default `8080`), `REDIS_HOST`, `REDIS_PORT`, plus `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` / `POSTGRES_PORT` for local Docker Compose.

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
│   └── platform/                 # config, database, redis, auth, logger, middleware
├── migrations/             # SQL migrations (golang-migrate)
└── test/integration/        # Testcontainers-based end-to-end tests
```

## Roadmap

This is intentionally a modular monolith today. The plan is to evolve it in stages:

1. **Modular Monolith** *(current)* — single Go service, clean domain boundaries.
2. **gRPC** — split order/inventory concerns into internal services.
3. **Event-driven** — introduce Kafka for order → payment/inventory/notification fan-out.
4. **Observability** — OpenTelemetry metrics, traces, and structured logs (Prometheus, Grafana, Jaeger).
5. **Load testing** — k6-driven load tests at 1000+ concurrent users, tracking P50/P95/P99 latency and error rate.
