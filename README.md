# Flash Sale Engine

A Go microservices backend for **flash sales** — short, high-traffic sales where a
limited quantity of stock is released and far more buyers arrive than there are
units available.

The problem a flash sale creates is not throughput in general, it is contention
on a single number: the remaining stock of one product. If every buyer's request
has to take a database lock on that row, the queue backs up, response times
collapse and the sale becomes a bad experience for everybody. If the locking is
relaxed to keep things fast, the sale oversells and someone has to cancel orders
after the fact.

This engine resolves that by splitting the sale into a **fast path** and a
**durable path**. Stock is decremented atomically in Redis, which answers in
microseconds and cannot oversell because the check-and-decrement runs as a single
Lua script inside Redis. The buyer gets an answer immediately. The slower work of
writing the order to PostgreSQL happens afterwards, driven by a RabbitMQ queue.

---

## Table of contents

- [Key features](#key-features)
- [Business value](#business-value)
- [Business use cases](#business-use-cases)
- [Architecture](#architecture)
- [Purchase flow](#purchase-flow)
- [Tech stack](#tech-stack)
- [Project structure](#project-structure)
- [Getting started](#getting-started)
- [Configuration](#configuration)
- [API reference](#api-reference)
- [Testing](#testing)
- [Development](#development)
- [Deployment](#deployment)
- [Operational notes and limitations](#operational-notes-and-limitations)

---

## Key features

Everything listed here is implemented in this repository.

| Feature | Where it lives | What it does |
| :--- | :--- | :--- |
| **Oversell-proof stock** | `purchase-service/redis` | Check-and-decrement runs as one Lua script inside Redis, so concurrent buyers can never take the counter below zero. |
| **JWT authentication** | `api-gateway/middleware`, `api-gateway/service` | HS256 tokens with a pinned algorithm and a required expiry. |
| **Cache-aside login** | `api-gateway/service/auth.go` | Credentials are read from Redis first and fall back to PostgreSQL, so a login spike does not hit the database on every request. |
| **Per-buyer idempotency** | `purchase-service/redis` | A short-lived claim per (buyer, product) stops a double-submitted order being placed twice. |
| **Asynchronous persistence** | `purchase-service/publisher`, `order-worker` | Accepted orders are published with publisher confirms and written to PostgreSQL by a separate worker. |
| **Compensating rollback** | `purchase-service/service` | If an order cannot be queued, the reserved stock is returned and the buyer's claim released, so no inventory is lost. |
| **Exactly-once cancellation** | `api-gateway/repository/order.go` | Cancellation and stock restoration commit in one transaction, guarded so concurrent cancellations cannot restore stock twice. |
| **Owner-scoped orders** | `api-gateway/repository/order.go` | Every order query is filtered by the authenticated buyer, so orders are private to their owner. |
| **Idempotent consumer** | `order-worker/repository` | Redelivered messages are recognised and ignored; messages that can never succeed are discarded rather than requeued forever. |
| **Per-client rate limiting** | `api-gateway/middleware/ratelimit.go` | A sliding window per client address, with a `Retry-After` header. |
| **Graceful shutdown** | all three services | In-flight requests and the message being processed are allowed to finish. |
| **Health endpoints** | `api-gateway`, `purchase-service` | Probes check the dependency the service cannot work without. |

---

## Business value

**The problem.** A flash sale concentrates a day's traffic into a few minutes
against a handful of products. Two things go wrong at that scale, and they pull
in opposite directions: a database that serialises every purchase becomes the
bottleneck and buyers see timeouts, while a system loose enough to stay fast
sells stock it does not have.

**What this engine provides.**

- **No overselling, and no manual cleanup.** Stock is decremented atomically, and
  the durable write is guarded by a stock floor in PostgreSQL as a second line of
  defence. The end-to-end test suite asserts this directly: 60 concurrent buyers
  against 15 units result in exactly 15 orders. Overselling is expensive in a way
  that is easy to underestimate — refunds, support contacts, and customers who
  were told they had bought something they had not.
- **Buyers get an answer immediately.** The response is returned as soon as stock
  is reserved in memory, rather than after a disk write. Sold-out is reported as
  fast as success, so nobody waits in a queue for a product that has already gone.
- **The sale stays up when the slow parts are busy.** Order writing is queued, so
  a slow database lengthens the queue rather than failing the sale. Logins are
  served from cache, so a login spike does not fall through to PostgreSQL on every
  request.
- **Lower infrastructure cost for the same peak.** The hot path touches Redis, not
  PostgreSQL, so the database is sized for the sustained order-writing rate rather
  than for peak purchase traffic.
- **Failures are visible, not silent.** When stock reservation and durable stock
  disagree, the order is recorded as `FAILED` rather than reported as successful,
  which leaves a record to reconcile instead of a discrepancy discovered later.
- **Operational effort stays low.** Retries, redeliveries and duplicate submissions
  are handled by the services, so a bad message or a restart does not need someone
  to intervene.

**Who benefits.** E-commerce and marketplace teams running limited drops;
ticketing platforms; teams releasing limited inventory such as vouchers, discount
codes or reservation slots; and engineers who need a worked reference for
Redis-fronted inventory with eventual persistence.

---

## Business use cases

**Limited product drop.** A retailer releases 500 units at 12:00. Traffic is
orders of magnitude higher than the stock. Buyers get an immediate accept or an
immediate sold-out; exactly 500 orders exist afterwards, and the operations team
has no post-sale reconciliation to do.

**Ticket release.** A venue opens a block of seats. The per-buyer idempotency
claim stops an impatient customer double-clicking their way into two orders,
without needing client-side protection. The rate limiter blunts scripted buying
from a single source.

**Voucher or discount-code campaign.** A marketing campaign issues a fixed number
of codes. The stock counter is the issuance budget, and it cannot be exceeded even
if the campaign is linked from several places at once.

**Cancellation within a sale window.** A buyer changes their mind. Cancelling
returns the unit to the live sale — to both the in-memory counter and the durable
record, in one transaction — so it can be bought by somebody else while the sale
is still running.

**Reference implementation.** A team evaluating a Redis-fronted inventory design
can run the whole system locally with one command and read the end-to-end tests as
an executable specification of the guarantees.

---

## Architecture

Three services, each owning one concern, over three pieces of infrastructure.

- **API gateway** is the only public entry point. It authenticates buyers, rate
  limits them, forwards purchases to the purchase service, and serves order reads
  and cancellations from PostgreSQL.
- **Purchase service** owns the hot path. It reserves stock in Redis and publishes
  an order event. It never writes to PostgreSQL during a sale — it reads product
  stock from PostgreSQL once at startup to warm the counters.
- **Order worker** consumes order events and writes each order to PostgreSQL,
  deducting durable stock in the same transaction.

The purchase service takes the buyer's identity from the request body, because the
gateway has already authenticated it. That makes it a trusted-caller service: it
is not published to the host in Docker Compose, and every one of its routes
requires a shared internal token.

```mermaid
graph LR
    Client(["Buyer"])

    AG["<b>API Gateway</b><br/>:8080<br/><i>auth · rate limit · orders</i>"]
    PS["<b>Purchase Service</b><br/>:8081<br/><i>reserve stock · publish</i>"]
    OW["<b>Order Worker</b><br/><i>persist orders</i>"]

    RD[("<b>Redis</b><br/>stock counters<br/>credential cache")]
    MQ{{"<b>RabbitMQ</b><br/>orders.queue"}}
    PG[("<b>PostgreSQL</b><br/>users · products · orders")]

    Client -->|"HTTPS<br/>signup · login · purchase<br/>read · cancel"| AG

    AG -->|"POST /purchase<br/>X-Internal-Token"| PS
    AG -->|"cache-aside<br/>credentials"| RD
    AG -->|"read orders<br/>cancel + restore stock<br/>(one transaction)"| PG

    PS -->|"atomic decrement<br/>idempotency claim"| RD
    PS -->|"publish order.created<br/>(publisher confirms)"| MQ
    PS -.->|"stock warm-up<br/>at startup only"| PG

    MQ -->|"consume<br/>one at a time"| OW
    OW -->|"insert order + deduct stock<br/>(one transaction)"| PG

    classDef edge fill:#1f7a4d,stroke:#0d4f2f,color:#ffffff
    classDef svc fill:#1d4f91,stroke:#12325c,color:#ffffff
    classDef store fill:#6b3fa0,stroke:#40265f,color:#ffffff
    classDef actor fill:#f0f0f0,stroke:#555555,color:#000000
    class Client actor
    class AG edge
    class PS,OW svc
    class RD,MQ,PG store
```

### Where each responsibility lives

| Concern | Owner |
| :--- | :--- |
| Authentication and token issuing | API gateway (`service/auth.go`) |
| Authorisation (order ownership) | API gateway (`repository/order.go`, enforced in SQL) |
| Rate limiting | API gateway (`middleware/ratelimit.go`) |
| Stock reservation and idempotency | Purchase service (`redis/`, `service/`) |
| Order event contract | Shared module (`common/models/order.go`) |
| Durable orders and stock | Order worker (`repository/`) and API gateway (cancellation) |

The `OrderEvent` contract lives in the shared `common` module and is imported by
both the publisher and the consumer, so the two cannot drift apart.

---

## Purchase flow

The flow below is the system's main business workflow, including the compensating
path that runs when an order cannot be queued.

```mermaid
sequenceDiagram
    autonumber
    actor Buyer
    participant AG as API Gateway
    participant PS as Purchase Service
    participant RD as Redis
    participant MQ as RabbitMQ
    participant OW as Order Worker
    participant PG as PostgreSQL

    Buyer->>AG: POST /purchase (Bearer token)
    AG->>AG: verify token, rate limit,<br/>take buyer id from claims
    AG->>PS: POST /purchase (X-Internal-Token)

    PS->>RD: claim (buyer, product)
    alt claim already held
        RD-->>PS: refused
        PS-->>AG: 409 duplicate
        AG-->>Buyer: 409 duplicate
    else claim taken
        PS->>RD: atomic check-and-decrement
        alt insufficient stock
            RD-->>PS: refused
            PS->>RD: release claim
            PS-->>AG: 409 out of stock
            AG-->>Buyer: 409 out of stock
        else reserved
            PS->>MQ: publish order.created
            alt broker did not confirm
                PS->>RD: return stock, release claim
                PS-->>AG: 503
                AG-->>Buyer: 503
            else confirmed
                PS-->>AG: 202 order id
                AG-->>Buyer: 202 order id, status PENDING
            end
        end
    end

    MQ->>OW: deliver order.created
    OW->>PG: BEGIN: insert order + deduct stock
    alt durable stock insufficient
        OW->>PG: record order as FAILED
    else committed
        OW->>PG: COMMIT (status SUCCESS)
    end
    OW->>MQ: ack

    Buyer->>AG: GET /orders/{id}
    AG->>PG: select where id AND user_id
    AG-->>Buyer: order with final status
```

---

## Tech stack

| Technology | Version | Role |
| :--- | :--- | :--- |
| **Go** | 1.23 | All services |
| **Gin** | v1.9.1 | HTTP routing and request binding |
| **Redis** | 7 (client `go-redis/v9`) | Stock counters, idempotency claims, credential cache |
| **RabbitMQ** | 3 (client `streadway/amqp`) | Order event queue, with publisher confirms |
| **PostgreSQL** | 17 (driver `lib/pq`) | Users, products, orders |
| **golang-jwt/jwt** | v5 | Access tokens |
| **golang.org/x/crypto** | bcrypt | Password hashing |
| **google/uuid** | v1.6.0 | Identifiers |
| **joho/godotenv** | v1.5.1 | Loading `.env` in local development |
| **Docker Compose** | v2 | Local orchestration |
| **GitHub Actions** | — | Format, vet, lint, test, end-to-end, image build |

The repository is a set of **six Go modules**. The three services and the seed
script each build independently, `common` is shared via a `replace` directive, and
`e2e` drives the built binaries.

---

## Project structure

```
.
├── .github/workflows/ci.yml     CI: format, vet, lint, tests, e2e, image build
└── flashsale/
    ├── common/                  Shared module (imported via a replace directive)
    │   ├── env/                 Environment variable helpers
    │   ├── models/              User, Product, Order, OrderEvent contract
    │   └── response/            The JSON envelope every endpoint returns
    ├── api-gateway/             Public HTTP entry point
    │   ├── main.go              Wiring and graceful shutdown
    │   ├── config/              Typed configuration with validation
    │   ├── router/              Route table
    │   ├── middleware/          JWT auth, rate limiting, request logging
    │   ├── handler/             Thin HTTP layer
    │   ├── service/             Business logic (auth, orders)
    │   ├── repository/          SQL for users and orders
    │   ├── client/              HTTP client for the purchase service
    │   └── dto/                 Request and response bodies
    ├── purchase-service/        Hot path: reserve stock, publish events
    │   ├── main.go
    │   ├── config/
    │   ├── router/
    │   ├── middleware/          Internal token guard
    │   ├── handler/
    │   ├── service/             Reservation and compensation logic
    │   ├── redis/               Stock counters and idempotency claims
    │   ├── publisher/           RabbitMQ publisher with confirms
    │   └── warmup/              Loads stock from PostgreSQL at startup
    ├── order-worker/            Consumes events, persists orders
    │   ├── main.go
    │   ├── config/
    │   ├── consumer/            AMQP transport and acknowledgement policy
    │   ├── repository/          Transactional order persistence
    │   └── postgres/            Connection pool
    ├── e2e/                     End-to-end tests driving the real binaries
    ├── scripts/seed.go          Idempotent demo data seeder
    ├── migrations/init.sql      Schema (idempotent, safe to re-run)
    ├── deployments/             One Dockerfile per service
    ├── docker-compose.yml       Full local stack
    ├── .golangci.yml            Lint configuration
    └── .env.example             Configuration template
```

---

## Getting started

### Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Docker](https://docs.docker.com/get-docker/) with Compose v2 — for the Docker route
- PostgreSQL 13+, Redis 7+ and RabbitMQ 3+ — for the manual route

### 1. Clone and configure

```bash
git clone https://github.com/albert4183r7/Flash-Sale-Engine.git
cd Flash-Sale-Engine/flashsale

cp .env.example .env
```

Then generate the two secrets and put them in `.env`:

```bash
openssl rand -hex 32   # -> JWT_SECRET
openssl rand -hex 32   # -> INTERNAL_API_TOKEN
```

The gateway refuses to start if `JWT_SECRET` is missing or shorter than 16
characters. That is deliberate: an empty signing secret would let anyone forge a
token for any account.

### 2a. Run with Docker Compose

```bash
# from flashsale/
docker compose up --build
```

Compose starts Redis, RabbitMQ and PostgreSQL, applies `migrations/init.sql` on
the database's first start, then starts the three services in dependency order.

Seed demo products and users in another terminal:

```bash
cd flashsale/scripts
go run .
```

The seeder prints the product IDs it wrote — those are the IDs you need to place
an order.

### 2b. Run without Docker

Start PostgreSQL, Redis and RabbitMQ yourself, then:

```bash
# from flashsale/
createdb -U postgres flashsale
psql -U postgres -d flashsale -f migrations/init.sql

cd scripts && go run . && cd ..
```

Run each service in its own terminal, from its own directory (each one loads
`../.env`):

```bash
cd flashsale/purchase-service && go run .   # terminal 1
cd flashsale/order-worker     && go run .   # terminal 2
cd flashsale/api-gateway      && go run .   # terminal 3
```

Start the purchase service before the gateway: the gateway forwards to it, and in
Compose it waits for its health check.

### 3. Verify

```bash
curl -s http://localhost:8080/health
```

### 4. Build binaries

```bash
# from flashsale/
for service in api-gateway purchase-service order-worker; do
  (cd "$service" && go build -o "../bin/$service" .)
done
```

---

## Configuration

All configuration comes from environment variables, loaded from `.env` in local
development. **Required** variables have no default: a service that is missing one
reports every missing variable at once and exits, rather than starting in a state
where it cannot work.

### Required

| Variable | Used by | Purpose |
| :--- | :--- | :--- |
| `JWT_SECRET` | gateway | Signs access tokens. Minimum 16 characters. **Secret.** |
| `INTERNAL_API_TOKEN` | gateway, purchase | Authenticates the gateway to the purchase service. **Secret.** |
| `POSTGRES_URL` | all three | PostgreSQL connection string. **Contains a password.** |
| `REDIS_ADDR` | gateway, purchase | Redis address, `host:port`. |
| `RABBITMQ_URL` | purchase, worker | AMQP connection string. **Contains a password.** |
| `PURCHASE_SERVICE_URL` | gateway | Base URL of the purchase service. |

### Optional

| Variable | Default | Purpose |
| :--- | :--- | :--- |
| `API_GATEWAY_PORT` | `8080` | Gateway listen port |
| `PURCHASE_SERVICE_PORT` | `8081` | Purchase service listen port |
| `REDIS_PASSWORD` | empty | Redis password, if the server requires one |
| `TOKEN_TTL` | `24h` | Access token lifetime |
| `CREDENTIAL_CACHE_TTL` | `1h` | How long a credential lookup stays cached |
| `IDEMPOTENCY_TTL` | `2m` | How long a buyer's claim on a product is held |
| `RATE_LIMIT_REQUESTS` | `120` | Requests allowed per client per window |
| `RATE_LIMIT_WINDOW` | `1m` | Rate limit window |
| `DB_TIMEOUT` | `10s` | Per-message database timeout in the worker |
| `SHUTDOWN_TIMEOUT` | `15s` | Grace period on shutdown |
| `SEED_PASSWORD` | `password123` | Password for the seeded demo accounts (development only) |

Never commit `.env` — it is git-ignored. `.env.example` contains placeholders
only.

---

## API reference

Every endpoint returns the same envelope:

```json
{ "success": true, "message": "...", "data": { }, "error": "..." }
```

`data` is present on success, `error` on failure.

### Public

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `GET` | `/health` | Liveness, including PostgreSQL reachability |
| `POST` | `/auth/signup` | Register an account |
| `POST` | `/auth/login` | Exchange credentials for a token |

### Authenticated — `Authorization: Bearer <token>`

| Method | Path | Purpose |
| :--- | :--- | :--- |
| `POST` | `/purchase` | Place an order |
| `GET` | `/orders/:id` | Read one of **your own** orders |
| `DELETE` | `/orders/:id` | Cancel one of **your own** orders and return its stock |

Orders are scoped to the authenticated buyer. Another user's order ID returns
`404`, the same as an ID that does not exist, so the API cannot be used to probe
for other buyers' orders.

### Status codes

| Code | Meaning |
| :--- | :--- |
| `200` | Read or cancellation succeeded |
| `201` | Account created |
| `202` | Purchase accepted and queued — the order is `PENDING`, not yet persisted |
| `400` | Malformed body or a value that failed validation |
| `401` | Missing, malformed, expired or wrongly signed token; bad credentials |
| `404` | Unknown product, or an order that is not yours |
| `409` | Duplicate purchase, out of stock, duplicate signup, already cancelled |
| `429` | Rate limit exceeded — see the `Retry-After` header |
| `500` | Unexpected server-side failure |
| `503` | A dependency was unreachable |

### Worked example

```bash
BASE=http://localhost:8080

# 1. Register
curl -s -X POST "$BASE/auth/signup" \
  -H 'Content-Type: application/json' \
  -d '{"email":"buyer@example.com","password":"password123"}'

# 2. Log in and keep the token
TOKEN=$(curl -s -X POST "$BASE/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"email":"buyer@example.com","password":"password123"}' \
  | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

# 3. Buy (use a product ID printed by the seeder)
PRODUCT_ID=<paste-from-seeder>
ORDER=$(curl -s -X POST "$BASE/purchase" \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"product_id\":\"$PRODUCT_ID\",\"qty\":1}")
echo "$ORDER"

ORDER_ID=$(echo "$ORDER" | grep -o '"order_id":"[^"]*"' | cut -d'"' -f4)

# 4. Read it back — status becomes SUCCESS once the worker has persisted it
curl -s "$BASE/orders/$ORDER_ID" -H "Authorization: Bearer $TOKEN"

# 5. Cancel it, returning the stock to the sale
curl -s -X DELETE "$BASE/orders/$ORDER_ID" -H "Authorization: Bearer $TOKEN"
```

On Windows PowerShell, use `curl.exe` and backticks for line continuation.

---

## Testing

The suite has three layers:

- **Unit tests** run anywhere, with no infrastructure. They cover the JWT
  middleware, the rate limiter, configuration validation, the purchase-service
  client, and the auth, order and purchase business logic against in-memory fakes.
- **Integration tests** need real PostgreSQL, Redis or RabbitMQ. They cover the
  things a mock cannot prove: SQL transactions and constraints, the atomicity of
  the Redis Lua script, and the consumer's acknowledgement policy.
- **End-to-end tests** (`e2e/`) build the three service binaries, run them as real
  processes, and drive the system through the gateway's public API.

Integration and end-to-end tests **skip themselves** when the infrastructure
environment variables are unset, so `go test ./...` works on a bare checkout.

```bash
# Unit tests only (no infrastructure needed)
cd flashsale/api-gateway && go test ./...

# Everything, with infrastructure available
cd flashsale
export POSTGRES_URL='postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable'
export REDIS_ADDR='localhost:6379'
export RABBITMQ_URL='amqp://guest:guest@localhost:5672/'
export JWT_SECRET='local-jwt-secret-at-least-16-chars'
export INTERNAL_API_TOKEN='local-internal-token'

for module in common api-gateway purchase-service order-worker; do
  (cd "$module" && go test -race -count=1 ./...)
done

# End-to-end: builds and runs the real binaries
(cd e2e && go test -count=1 -timeout 10m ./...)
```

### Race detection

The stock counters, the AMQP publisher channel and the rate limiter are all
shared state under concurrent load, so `-race` is used throughout and in CI.

### What the concurrency tests assert

| Test | Guarantee |
| :--- | :--- |
| `redis.TestDecrementStockNeverOversells` | 300 concurrent buyers against 50 units sell exactly 50 |
| `e2e.TestConcurrentBuyersNeverOversell` | 60 concurrent buyers against 15 units produce exactly 15 persisted orders |
| `repository.TestOrderRepositoryCancelIsExactlyOnceUnderConcurrency` | 8 concurrent cancellations restore stock exactly once |
| `middleware.TestRateLimitConcurrentAccess` | The limit holds exactly under concurrent requests |
| `consumer.TestConsumerDiscardsPermanentFailureAndKeepsGoing` | An unprocessable message is discarded, not requeued forever |

### Static analysis

```bash
cd flashsale
gofmt -l .                                   # must print nothing

for module in common api-gateway purchase-service order-worker scripts e2e; do
  (cd "$module" && go vet ./... && golangci-lint run --config ../.golangci.yml ./...)
done
```

---

## Development

```bash
cd flashsale

# Format
gofmt -w .

# Bring up only the infrastructure, and run the services from source
docker compose up -d redis rabbitmq postgres

# Re-seed (idempotent: it resets stock rather than duplicating products)
cd scripts && go run .

# Watch the queue
docker compose exec rabbitmq rabbitmqctl list_queues name messages

# Inspect a stock counter
docker compose exec redis redis-cli get "product_stock:<product-id>"
```

The RabbitMQ management UI is at <http://localhost:15672> (`guest` / `guest` by
default).

### Working on the shared module

`common` is consumed through a `replace` directive, so a change to it is picked up
by the services with no version bump. The service Dockerfiles copy `common/`
alongside the service for the same reason — the build context is `flashsale/`, not
the service directory.

---

## Deployment

`docker-compose.yml` builds and runs the full stack. Each service has its own
Dockerfile in `deployments/`, producing a static binary in a minimal Alpine image
that runs as an unprivileged user.

```bash
cd flashsale
docker compose up --build -d
docker compose ps
docker compose logs -f api-gateway
docker compose down          # add -v to drop the data volumes
```

Notes for anything beyond a local run:

- **Only the gateway should be publicly reachable.** The purchase service is not
  published to the host in Compose; it is reached over the internal network and
  its routes require `INTERNAL_API_TOKEN`.
- **Replace both secrets.** `JWT_SECRET` and `INTERNAL_API_TOKEN` must be
  generated per environment and supplied by a secret manager, not baked into an
  image or committed.
- **Terminate TLS in front of the gateway.** The services speak plain HTTP.
- **`migrations/init.sql` runs only on an empty data directory.** Apply schema
  changes to an existing database yourself; the file is written to be safe to
  re-run.
- **The order worker scales horizontally.** Run more replicas to consume faster;
  each takes one message at a time and duplicate delivery is handled.
- **The gateway scales horizontally with one caveat**, described below.

---

## Operational notes and limitations

These are real properties of the current implementation, worth knowing before
running it in production.

- **Rate limiting is per process.** Each gateway instance keeps its own counters,
  so N instances allow up to N times the configured limit for one client. A shared
  Redis-backed limiter would be needed for a strict global limit.
- **Redis is the source of truth during a sale.** Stock counters are seeded from
  PostgreSQL when the purchase service starts. If Redis loses its data, restart the
  purchase service to warm the counters again. Products created while it is running
  are not picked up until the next restart.
- **Persistence is eventual.** A purchase returns `202` with status `PENDING`. The
  order becomes `SUCCESS` once the worker has written it, normally within
  milliseconds, but a client must not treat `202` as "persisted".
- **A cancellation credits PostgreSQL first, then Redis.** If the Redis update
  fails afterwards, the cancellation still stands and the in-memory counter is
  temporarily low. That under-sells rather than oversells, is logged as
  `CRITICAL`, and is corrected at the next warm-up.
- **Login caches the password hash in Redis.** That is what makes cache-aside
  logins possible. The hash is bcrypt and never leaves the server, but Redis should
  be treated as holding credential material: require a password, and do not expose
  it.
- **`FAILED` orders need reconciliation.** They are recorded when Redis allowed a
  reservation that durable stock could not cover, which should not happen in normal
  operation and indicates the two have diverged.

---

## License

MIT — free to use for portfolio or personal projects.
