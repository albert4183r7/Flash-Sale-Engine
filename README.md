# ⚡ Flash Sale System

A professional-grade flash sale backend engine built with **Go** microservices architecture. This project demonstrates distributed systems concepts including Microservices, JWT Authentication, Redis-based stock management, and Eventual Consistency.

## 📂 Project Structure

```
flashsale/
├── 🚪 api-gateway/          \# Gin-based HTTP gateway (Auth, Routing)
├── ⚡ purchase-service/     \# High-speed Logic (Redis, RabbitMQ Publisher)
├── 👷 order-worker/         \# Async Worker (Postgres, RabbitMQ Consumer)
├── 🔧 common/               \# Shared utilities
├── 📜 migrations/           \# SQL Schemas
├── 🌱 scripts/              \# Utility scripts (Seeding)
├── 🐳 deployments/          \# Docker configurations
├── 🐙 docker-compose.yml    \# Full stack orchestration
└── 📖 README.md             \# Documentation
```

## 🏗️ System Architecture

### 🚀 How It Works

Imagine a crowded concert ticket sale. If everyone pushes the ticket clerk at once, the line stops moving. This system solves that problem by splitting the work into three distinct roles:

1.  **🕵️ The Bouncer (API Gateway):** Checks your ID at the door. If you aren't on the list (logged in), you don't get in.
2.  **🏎️ The Fast Cashier (Purchase Service):** Instead of writing a full contract, this cashier just checks a simple tally board (**Redis**).
    * *Stock available?* They hand you a "Reserved" ticket instantly.
    * *Stock empty?* They say "Sold Out" immediately.
3.  **🗄️ The Back Office (Order Worker):** A separate team takes the "Reserved" tickets from a pile (**RabbitMQ**) and quietly files the official paperwork (**PostgreSQL**) in the background, so you don't have to wait for the filing cabinet to open.

---

### ⚙️ Technical Workflow

The system optimizes for high concurrency by offloading writes to memory:

**1. 🔐 Authentication (API Gateway):** The entry point. It validates **JWT Tokens** to ensure the user is legit and applies **Rate Limiting** to prevent spambots.

**2. 🧠 In-Memory Locking (Purchase Service):** The core engine. It talks to **Redis** (RAM-based storage) using atomic Lua scripts. This ensures that even if 1,000 requests hit at the exact same millisecond, the stock count is accurate and no double-booking occurs.

**3. 📨 Async Processing (RabbitMQ):** Once stock is reserved in Redis, the system returns `202 Accepted` to the user immediately. It then publishes an event to a Message Queue.

**4. 💾 Eventual Consistency (Order Worker):** A background worker consumes the message and performs the heavier operation of inserting the order into **PostgreSQL** (Disk-based storage), ensuring data persistence without blocking the user's response time.

```mermaid
graph TD
    %% Nodes
    User((👤 User))
    AG[🚪 API Gateway]
    PS[⚡ Purchase Service]
    W[👷 Order Worker]
    
    subgraph Data_Layer [💾 Data Persistence & Queue]
        DB[(💾 PostgreSQL)]
        R[(🧠 Redis Cache)]
        MQ[📨 RabbitMQ]
    end

    %% 1. Authentication Flow
    User ==>|1. Login / Signup| AG
    AG <==>|2. Verify Credentials| DB
    AG ==>|3. Return JWT Token| User

    %% 2. Purchase Flow
    User ==>|4. POST /purchase + Token| AG
    AG ==>|5. Validate & Forward| PS
    PS <==>|6. Atomic Stock Decr| R
    
    %% 3. Async Processing
    PS ==>|7. Publish Event| MQ
    MQ ==>|8. Consume Message| W
    W ==>|9. Insert Order & Sync| DB
    
    %% 4. Cancellation Flow (Dotted)
    User -.->|10. DELETE /order| AG
    AG -.->|11. Soft Delete| DB
    AG -.->|12. Request Restore| PS
    PS -.->|13. Increment Stock| R

    %% Styling
    linkStyle 0,1,2,3,4,5,6,7,8 stroke:#2ecc71,stroke-width:2px;
    linkStyle 9,10,11,12 stroke:#e74c3c,stroke-width:2px,stroke-dasharray: 5 5;
````

## 🛠️ Core Technologies

| Technology | Purpose |
| :--- | :--- |
| **Go 1.23** | Fast, concurrent, simple syntax |
| **Gin** | Lightweight HTTP framework |
| **Redis** | In-memory stock & idempotency |
| **RabbitMQ** | Async message processing |
| **PostgreSQL** | Persistent order storage |
| **Docker Compose** | Local orchestration |


## 🏁 Quick Start

### Option 1: 🐳 With Docker Compose (Recommended)

This automates the entire setup, including database creation and schema initialization.

```bash
# 1. Setup Environment Variables
# Create the .env file from the example template
cp .env.example .env

# 2. Start all services
cd flashsale && docker-compose up --build

# 3. Seed the database (In a new terminal)
# This populates initial products and users
cd scripts && go run seed.go
```

-----

### Option 2: 🛠️ Without Docker (Manual Setup)

#### Prerequisites (Windows)

  - [Go 1.23+](https://go.dev/dl/)
  - [Redis](https://github.com/tporadowski/redis/releases)
  - [RabbitMQ](https://www.rabbitmq.com/download.html)
  - [PostgreSQL](https://www.postgresql.org/download/windows/)

#### Database Setup

You must manually create the database and run the schema file.

**PowerShell:**

```powershell
# 1. Start Services
net start redis
net start rabbitmq
net start postgresql-x64-15

# 2. Create Database
psql -U postgres -c "CREATE DATABASE flashsale;"

# 3. Initialize Schema (Tables)
psql -U postgres -d flashsale -f migrations/init.sql

# 4. Setup Config
# Manually create .env or set environment variables in your terminal session

# 5. Seed Data
cd flashsale\scripts
go run seed.go
```

#### Run Microservices

Open 3 separate terminals:

**Terminal 1 (API Gateway):**

```powershell
cd flashsale\api-gateway; go run main.go
```

**Terminal 2 (Purchase Service):**

```powershell
cd flashsale\purchase-service; go run main.go
```

**Terminal 3 (Order Worker):**

```powershell
cd flashsale/order-worker; go run main.go
```


## 🧪 API Endpoints & Testing Guide

Use `curl` commands to test the system.

### 1\. 📝 Sign Up

Create a new user account.

```bash
curl.exe -X POST http://localhost:8080/auth/signup `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "password123"}'
```

**Example Output:**

```json
{
  "message": "User registered successfully"
}
```

-----

### 2\. 🔑 Login

Authenticate and receive a JWT token.

```bash
$response = curl.exe -s -X POST "http://localhost:8080/auth/login" `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "password123"}'

# Parse JSON to get Token
$json = $response | ConvertFrom-Json
$TOKEN = $json.data.token

# Display Full Response
$json | ConvertTo-Json -Depth 5
```

**Example Output:**

```json
{
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.***HIDDEN***",
    "expires_in": 86400,
    "user_id": 4,
    "email": "tester@example.com",
    "role": "user"
  }
}
```

-----

### 3\. 🛍️ Purchase Item

Buy a product (e.g., ID 1: iPhone 15 Pro).

```bash
$response = curl.exe -s -X POST "http://localhost:8080/purchase" `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": 1, "qty": 1}'

# Capture Order ID
$json = $response | ConvertFrom-Json
$ORDER_ID = $json.data.order_id
echo "Order Created: $ORDER_ID"

# Display Full Response
$json | ConvertTo-Json -Depth 5  
```

**Example Output:**

```json
Order Created: 9096de05-4396-4861-9208-0a92e9a145e2
{
  "data": {
    "order_id": "9096de05-4396-4861-9208-0a92e9a145e2",
    "status": "PENDING",
    "message": "Your order is being processed",
    "product_id": 1,
    "qty": 1
  },
  "message": "Purchase accepted and queued for processing",
  "success": true
}
```

-----

### 4\. 🔍 Check Order Status

```bash
curl.exe -X GET http://localhost:8080/orders/$ORDER_ID `
  -H "Authorization: Bearer $TOKEN"
```

**Example Output:**

```json
{
  "order_id":"9096de05-4396-4861-9208-0a92e9a145e2",
  "status":"SUCCESS"
}
```

-----

### 5\. 🚫 Cancel Order

Cancels the order and restores stock.

```bash
# 5. Cancel order
curl.exe -X DELETE http://localhost:8080/orders/$ORDER_ID `
  -H "Authorization: Bearer $TOKEN"
```

**Example Output:**

```json
{
  "message": "Order cancelled and stock restored"
}
```

## ⚠️ Failure Test Cases

### Test Case A: 🔁 Duplicate Purchase (Idempotency)

Try buying the same product again immediately.

```bash
curl.exe -X POST http://localhost:8080/purchase `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": 1, "qty": 1}'
```

**Example Output:**

```json
{
  "success": false,
  "message": "Duplicate purchase request",
  "error": "You already have a pending purchase for product 1"
}
```

-----

### Test Case B: 📉 Out of Stock

Try buying more than available stock (e.g., 500 units).

```bash
curl.exe -X POST http://localhost:8080/purchase `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": 2, "qty": 500}'
```

**Example Output:**

```json
{
  "success": false,
  "message": "Out of stock",
  "error": "Product 2 is out of stock"
}
```

-----

### Test Case C: ⛔ Unauthorized

Try accessing without a token.

```bash
curl.exe -X POST http://localhost:8080/purchase `
  -H "Content-Type: application/json" `
  -d '{"product_id": 1, "qty": 1}'
```

**Example Output:**

```json
{
  "success": false,
  "message": "Authorization required",
  "error": "Missing Authorization header"
}
```

-----

### Test Case D: ⏳ Rate Limiting

Spam the API with requests (Simulate loop).

```bash
for($i=1; $i -le 105; $i++) {
  curl.exe -s -o $null -X POST http://localhost:8080/purchase `
    -H "Authorization: Bearer $TOKEN" `
    -H "Content-Type: application/json" `
    -d '{"product_id": 1, "qty": 1}'
}
```

**Example Output:**

```json
{
  "success": false,
  "message": "Rate limit exceeded",
  "error": "Too many requests. Please try again later."
}
```

-----

### Test Case E: ❌ Invalid Credentials

Login with wrong password.

```bash
curl.exe -X POST http://localhost:8080/auth/login `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "wrong"}'
```

**Example Output:**

```json
{
  "error": "Invalid credentials"
}
```

## 💾 Database Schema

```sql
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'user'
);

CREATE TABLE products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price INT NOT NULL,
    stock INT NOT NULL CHECK (stock >= 0)
);

CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id),
    product_id INT NOT NULL REFERENCES products(id),
    qty INT NOT NULL,
    status VARCHAR(20) DEFAULT 'PENDING',
    created_at TIMESTAMP DEFAULT NOW()
);
```

## 📄 License

MIT - Use for portfolio or personal projects!