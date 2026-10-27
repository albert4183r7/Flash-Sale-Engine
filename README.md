# ⚡ Flash Sale Engine

A flash sale backend built with **Go microservices**. In a flash sale a small
amount of stock meets far more buyers than it can serve, all at the same moment.
This engine answers every buyer immediately, **never oversells**, and **never
creates the same order twice**.

It covers these distributed-systems concepts: microservices, JWT
authentication, atomic stock control in Redis, asynchronous processing with a
message queue, idempotency, and eventual consistency.

## 🧩 What the System Is Made Of

**3 Go services**

| Service | Role |
| :--- | :--- |
| 🚪 **API Gateway** (`:8080`) | The only public entry point. Handles signup/login (JWT), rate limiting, forwarding purchases, and reading or cancelling orders. |
| ⚡ **Purchase Service** (`:8081`, internal) | The hot path. Checks idempotency, reserves stock atomically in Redis, and publishes an order event to RabbitMQ. Only the gateway can call it. |
| 👷 **Order Worker** | A background consumer. Reads order events from RabbitMQ and writes each order to PostgreSQL. |

**3 pieces of infrastructure**

| Component | Used for |
| :--- | :--- |
| 🧠 **Redis** | Live stock counters, per-buyer idempotency keys, and the login cache |
| 📨 **RabbitMQ** | Order queue between the purchase service and the worker |
| 💾 **PostgreSQL** | Durable storage for users, products and orders |

## 📂 Project Structure

```
flashsale/
├── 🚪 api-gateway/          # Public HTTP API: auth, rate limit, orders
├── ⚡ purchase-service/     # Redis stock reservation + RabbitMQ publisher
├── 👷 order-worker/         # RabbitMQ consumer + PostgreSQL writer
├── 🔧 common/               # Shared code: config, models, Postgres & RabbitMQ setup
├── 🧪 e2e/                  # End-to-end tests that run the real services
├── 📜 migrations/           # SQL schema
├── 🌱 scripts/              # Demo data seeder
├── 🐳 deployments/          # Dockerfiles
├── 🐙 docker-compose.yml    # Full stack orchestration
└── 🔑 .env.example          # Configuration template
```

## 🏗️ System Architecture

### 🚀 How It Works

Think of a crowded concert ticket sale. If everyone pushes the ticket clerk at
once, the line stops moving. This system splits the work into three roles:

1. **🕵️ The Bouncer (API Gateway):** checks your ID at the door. Without a valid
   login token, you don't get in.
2. **🏎️ The Fast Cashier (Purchase Service):** doesn't write a full contract. It
   only checks a tally board (**Redis**).
   - *Stock available?* You get a "Reserved" ticket instantly.
   - *Stock empty?* You hear "Sold Out" instantly.
3. **🗄️ The Back Office (Order Worker):** a separate team takes the reserved
   tickets from a pile (**RabbitMQ**) and files the official paperwork
   (**PostgreSQL**) in the background, so you never wait on the filing cabinet.

```mermaid
graph LR
    User((👤 Buyer))
    AG[🚪 API Gateway<br/>JWT · rate limit]
    PS[⚡ Purchase Service<br/>reserve stock]
    W[👷 Order Worker]

    subgraph Data [💾 Data & Queue]
        R[(🧠 Redis)]
        MQ[[📨 RabbitMQ]]
        DB[(💾 PostgreSQL)]
    end

    User -->|HTTP + JWT| AG
    AG -->|login cache| R
    AG -->|users, orders| DB
    AG -->|internal token| PS
    PS -->|atomic stock + idempotency| R
    PS -->|order.created| MQ
    MQ --> W
    W -->|insert order + deduct stock| DB
```

### ⚙️ Technical Workflow

1. **🔐 Login with a cache-aside pattern.** The gateway looks up credentials in
   Redis first. On a miss it reads PostgreSQL and caches the result for 1 hour,
   so a login spike does not overload the database.
2. **🧠 Atomic stock reservation.** The purchase service checks and decrements
   stock in one **Lua script** inside Redis. Redis runs the script as a single
   step, so even 1,000 requests in the same millisecond cannot take stock below
   zero.
3. **🔁 Idempotency.** Before reserving stock, the service sets a Redis key
   `purchase:{user}:{product}` with `SETNX` and a 2-minute TTL. A double-click or
   resubmission finds the key already set and gets `409 Duplicate purchase`.
4. **📨 Asynchronous processing.** Once stock is reserved, the order event is
   published to RabbitMQ with publisher confirms, and the buyer gets
   `202 Accepted` right away. If publishing fails, the reserved stock and the
   idempotency key are rolled back.
5. **💾 Eventual consistency.** The order worker inserts the order and deducts
   stock in PostgreSQL in a single transaction. `ON CONFLICT (id) DO NOTHING`
   makes a redelivered message harmless, so RabbitMQ's at-least-once delivery
   never creates a duplicate order.
6. **🚫 Exactly-once cancellation.** Cancelling flips the order status and returns
   its stock in one transaction. Only one request can ever cancel a given order,
   so stock is never returned twice.

## 🔄 User Flow

```mermaid
sequenceDiagram
    actor U as 👤 Buyer
    participant AG as 🚪 API Gateway
    participant PS as ⚡ Purchase Service
    participant R as 🧠 Redis
    participant MQ as 📨 RabbitMQ
    participant W as 👷 Order Worker
    participant DB as 💾 PostgreSQL

    U->>AG: POST /auth/signup
    U->>AG: POST /auth/login
    AG->>R: cached credentials?
    AG->>DB: (on cache miss) read user
    AG-->>U: JWT token

    U->>AG: POST /purchase (Bearer token)
    AG->>PS: forward (buyer ID from token)
    PS->>R: SETNX idempotency key
    PS->>R: Lua: check & decrement stock
    PS->>MQ: publish order.created
    AG-->>U: 202 Accepted, status PENDING

    MQ->>W: deliver event
    W->>DB: insert order + deduct stock (1 transaction)

    U->>AG: GET /orders/:id
    AG-->>U: status SUCCESS

    U->>AG: DELETE /orders/:id
    AG->>DB: mark CANCELLED + restore stock (1 transaction)
    AG->>PS: restore stock in Redis
    AG-->>U: 200 cancelled
```

**Order status lifecycle:** `PENDING` (accepted, queued) → `SUCCESS` (stored by
the worker) → `CANCELLED` (cancelled by the buyer). An order is stored as
`FAILED` when PostgreSQL's stock cannot cover it, which flags a Redis/DB mismatch
for reconciliation.

## 🛠️ Core Technologies

| Technology | Purpose |
| :--- | :--- |
| **Go 1.23** | Fast, concurrent services |
| **Gin** | HTTP framework |
| **Redis 7** | Stock counters, idempotency keys, login cache |
| **RabbitMQ 3** | Asynchronous order queue |
| **PostgreSQL 17** | Persistent storage |
| **JWT (HS256)** | Stateless authentication |
| **Docker Compose** | Local orchestration |
| **GitHub Actions** | CI: lint, tests, Docker builds |

## 📡 API Endpoints

All responses share one envelope:
`{"success": bool, "message": "...", "data": {...}, "error": "..."}`

| Method | Endpoint | Auth | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/health` | – | Health check |
| `POST` | `/auth/signup` | – | Register: `{"email", "password"}` (password 8–72 chars) |
| `POST` | `/auth/login` | – | Log in and receive a JWT |
| `POST` | `/purchase` | Bearer | Buy: `{"product_id", "qty"}` (qty 1–10) |
| `GET` | `/orders/:id` | Bearer | Get one of **your own** orders |
| `DELETE` | `/orders/:id` | Bearer | Cancel one of **your own** orders and restore its stock |

| Status | Meaning |
| :--- | :--- |
| `200` / `201` / `202` | OK / account created / purchase accepted (`PENDING`) |
| `400` | Invalid body or value |
| `401` | Missing or invalid token, wrong credentials |
| `404` | Unknown product, or an order that is not yours |
| `409` | Duplicate purchase, out of stock, email taken, already cancelled |
| `429` | Rate limit exceeded (120 requests/minute per client) |
| `503` | A dependency is unavailable |

## 🏁 Setup

### 1. ⚙️ Configure

```bash
git clone https://github.com/albert4183r7/Flash-Sale-Engine.git
cd Flash-Sale-Engine/flashsale
cp .env.example .env
```

Fill in the four blank values in `.env`:

| Variable | Value |
| :--- | :--- |
| `POSTGRES_PASSWORD`, `RABBITMQ_PASSWORD` | Any value you choose |
| `JWT_SECRET`, `INTERNAL_API_TOKEN` | Generate each with `openssl rand -hex 32` |

### 2a. 🐳 Run with Docker Compose (recommended)

```bash
# from flashsale/
docker compose up --build -d          # Redis, RabbitMQ, PostgreSQL + 3 services
                                      # (the schema is applied automatically)

cd scripts && go run . && cd ..       # seed demo products & users, prints product IDs
docker compose restart purchase-service   # reload Redis stock from the seeded products
```

> The purchase service copies stock from PostgreSQL into Redis **at startup**,
> so restart it after seeding or after changing stock in the database.

### 2b. 🛠️ Run without Docker

Requirements: Go 1.23+, PostgreSQL 13+, Redis 7+ and RabbitMQ 3+ running
locally. Set `POSTGRES_PASSWORD`, `RABBITMQ_USER` and `RABBITMQ_PASSWORD` in
`.env` to match your local installation.

```bash
# from flashsale/
psql -U postgres -c "CREATE DATABASE flashsale;"
psql -U postgres -d flashsale -f migrations/init.sql
cd scripts && go run . && cd ..
```

Then start each service in its own terminal, from its own folder. Each one
reads `../.env`.

```bash
cd purchase-service && go run .   # terminal 1 (start this one first)
cd order-worker     && go run .   # terminal 2
cd api-gateway      && go run .   # terminal 3
```

### 3. ✅ Verify

```bash
curl -s http://localhost:8080/health
```

## 🧪 Try It: Happy Path

The examples use bash and [`jq`](https://jqlang.github.io/jq/). On Windows,
use Git Bash or WSL.

```bash
BASE=http://localhost:8080
EMAIL="buyer-$RANDOM@example.com"
CRED=$(openssl rand -hex 12)     # a random password for the demo account
PRODUCT_ID=<id of "iPhone 15 Pro" printed by the seeder>
```

**1. 📝 Sign up**

```bash
curl -s -X POST $BASE/auth/signup -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$CRED\"}"
```

```json
{ "success": true, "message": "User registered successfully" }
```

**2. 🔑 Log in and keep the token**

```bash
TOKEN=$(curl -s -X POST $BASE/auth/login -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$CRED\"}" | jq -r .data.token)
```

```json
{
  "success": true,
  "message": "Login successful",
  "data": { "token": "eyJhbGciOi...", "expires_in": 86400, "user_id": "…", "email": "…", "role": "user" }
}
```

**3. 🛍️ Purchase**

```bash
ORDER_ID=$(curl -s -X POST $BASE/purchase -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"product_id\":\"$PRODUCT_ID\",\"qty\":1}" | tee /dev/stderr | jq -r .data.order_id)
```

```json
{
  "success": true,
  "message": "Purchase accepted and queued for processing",
  "data": { "order_id": "…", "status": "PENDING", "product_id": "…", "qty": 1 }
}
```

**4. 🔍 Check the order**

The status becomes `SUCCESS` once the worker has stored the order.

```bash
curl -s $BASE/orders/$ORDER_ID -H "Authorization: Bearer $TOKEN"
```

```json
{
  "success": true,
  "message": "Order retrieved",
  "data": { "order_id": "…", "user_id": "…", "product_id": "…", "qty": 1, "status": "SUCCESS", "created_at": "…" }
}
```

**5. 🚫 Cancel the order**

```bash
curl -s -X DELETE $BASE/orders/$ORDER_ID -H "Authorization: Bearer $TOKEN"
```

```json
{ "success": true, "message": "Order cancelled and stock restored", "data": { "status": "CANCELLED", "…": "…" } }
```

## ⚠️ Try It: Failure Cases

Run these after the happy path, using the same variables.

| # | Case | Request | Expected |
| :--- | :--- | :--- | :--- |
| A | 🔁 Duplicate purchase (within the 2-minute idempotency window) | Repeat step 3 | `409` `Duplicate purchase request` |
| B | 📉 Out of stock | Buy `qty: 9` of **MacBook Air M3** (seeded with 8) | `409` `Out of stock` |
| C | 🔢 Invalid quantity | `qty: 0` or `qty: 11` | `400` `Invalid request` |
| D | ⛔ No token | `POST /purchase` without `Authorization` | `401` `Authorization required` |
| E | ❌ Wrong password | Log in with a wrong password | `401` `Login failed` / `Invalid credentials` |
| F | 🔒 Someone else's order | `GET /orders/$ORDER_ID` with another user's token | `404` `Order not found` |
| G | 🚫 Cancel twice | Repeat step 5 | `409` `Order cannot be cancelled` |
| H | ⏳ Rate limit | 130 requests within a minute | `429` `Rate limit exceeded` |

```bash
# A. duplicate purchase
curl -s -X POST $BASE/purchase -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d "{\"product_id\":\"$PRODUCT_ID\",\"qty\":1}"

# B. out of stock
MACBOOK_ID=<id of "MacBook Air M3" printed by the seeder>
curl -s -X POST $BASE/purchase -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d "{\"product_id\":\"$MACBOOK_ID\",\"qty\":9}"

# D. no token
curl -s -X POST $BASE/purchase -H "Content-Type: application/json" \
  -d "{\"product_id\":\"$PRODUCT_ID\",\"qty\":1}"

# H. rate limit: print the status code of each request
for i in $(seq 1 130); do curl -s -o /dev/null -w "%{http_code} " $BASE/health; done; echo
```

## 🤖 Automated Tests

Each module is tested separately. Unit tests need no infrastructure:

```bash
cd flashsale/<module> && go test -race ./...
# <module>: common | api-gateway | purchase-service | order-worker | e2e
```

Integration and end-to-end tests need real PostgreSQL, Redis and RabbitMQ, and
**skip themselves** when these are not configured. To run everything locally:

```bash
# from flashsale/, with the infrastructure running (docker compose up -d redis rabbitmq postgres)
set -a; source .env; set +a
export POSTGRES_URL="postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@localhost:5432/$POSTGRES_DB?sslmode=disable"
export RABBITMQ_URL="amqp://$RABBITMQ_USER:$RABBITMQ_PASSWORD@localhost:5672/"

for m in common api-gateway purchase-service order-worker; do (cd $m && go test -race ./...); done
cd e2e && go test -count=1 -timeout 10m ./...
```

The end-to-end suite builds the three services, runs them on ports
18080/18081, and drives them over HTTP. Together with the integration tests,
the suite checks:

| Area | Test cases |
| :--- | :--- |
| 🛒 Purchase | Order is persisted and stock deducted in Redis and PostgreSQL; invalid requests rejected; refused when stock is short |
| 🔥 Concurrency | **60 concurrent buyers vs 15 units → exactly 15 orders**, no oversell |
| 🔁 Idempotency | Duplicate purchase refused; allowed again after the TTL expires |
| 🚫 Cancellation | Stock restored in both stores; exactly once even under concurrent cancels |
| 🔒 Security | Protected routes need a token; orders are private to their owner; purchase service rejects callers without the internal token |
| 🔑 Auth | Bad credentials rejected; duplicate and weak signups rejected |
| 📨 Worker | Redelivered messages ignored; bad messages discarded; transient DB failures retried |

CI (`.github/workflows/ci.yml`) runs formatting, `go vet`, `golangci-lint`, all
tests with `-race`, and the Docker image builds on every push and pull request.

## 💾 Database Schema

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role          VARCHAR(50) NOT NULL DEFAULT 'user',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE products (
    id    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name  VARCHAR(255) NOT NULL UNIQUE,
    price INT NOT NULL CHECK (price >= 0),
    stock INT NOT NULL CHECK (stock >= 0)        -- durable guard against overselling
);

CREATE TABLE orders (
    id         UUID PRIMARY KEY,                 -- order_id, generated by the purchase service
    user_id    UUID NOT NULL REFERENCES users(id),
    product_id UUID NOT NULL REFERENCES products(id),
    qty        INT NOT NULL CHECK (qty > 0),
    status     VARCHAR(20) NOT NULL DEFAULT 'PENDING'
               CHECK (status IN ('PENDING', 'SUCCESS', 'FAILED', 'CANCELLED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

The full, re-runnable schema with indexes is in
[`flashsale/migrations/init.sql`](flashsale/migrations/init.sql).

## 📄 License

MIT. Free to use for portfolio or personal projects.
