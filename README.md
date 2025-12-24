# ⚡ Flash Sale System

A flash sale backend engine built with **Go** microservices architecture. This project demonstrates distributed systems concepts including Microservices, JWT Authentication, Redis-based stock management, and Eventual Consistency.

## 📂 Project Structure

```
flashsale/
├── 🚪 api-gateway/          # Pure routing/proxy layer (JWT validation, rate limiting)
├── 👤 user-service/         # Authentication & user management
├── 📦 product-service/      # Product catalog & stock management
├── 📋 order-service/        # Order CRUD & status management
├── ⚡ purchase-service/     # High-speed purchase logic (Redis, RabbitMQ)
├── � payment-service/      # Mock payment processing
├── �👷 order-worker/         # Async order processing (RabbitMQ Consumer)
├── 📜 migrations/           # Service-specific SQL schemas
├── 🌱 scripts/              # Utility scripts (Seeding)
├── 🐳 deployments/          # Docker configurations
├── 🐙 docker-compose.yml    # Full stack (7 services, 4 databases)
└── 📖 README.md             # Documentation
```

## 🏗️ System Architecture

### 🚀 How It Works

Imagine a crowded concert ticket sale. If everyone pushes the ticket clerk at once, the line stops moving. This system solves that problem by splitting the work into six distinct microservices:

1.  **🚪 API Gateway:** Routes requests and validates JWT tokens. Pure proxy with no database access.
2.  **👤 User Service:** Handles authentication, JWT generation, and user management.
3.  **📦 Product Service:** Manages product catalog and Redis-based stock operations.
4.  **📋 Order Service:** Stores and manages order lifecycle.
5.  **⚡ Purchase Service:** High-speed purchase logic using Redis atomic operations.
6.  **� Payment Service:** Mock payment processing for development/testing.
7.  **�👷 Order Worker:** Async background worker that processes orders and payments.

---

### ⚙️ Technical Workflow

The system optimizes for high concurrency by offloading reads and writes to memory:

1. **🔐 Authentication (User Service)**: User service handles login/signup with JWT tokens. User data is cached in Redis for fast lookups.

2. **🧠 In-Memory Locking (Purchase Service)**: The core engine. Uses **Redis** with atomic Lua scripts to ensure accurate stock counts even under 1,000+ concurrent requests.

3. **📨 Async Processing (RabbitMQ)**: Once stock is reserved in Redis, the system returns `202 Accepted` immediately and publishes an event to RabbitMQ.

4. **💾 Eventual Consistency (Order Worker)**: A background worker consumes messages and persists orders to **PostgreSQL** via the Order Service.


```mermaid
graph TD
    User((👤 User)) --> AG[🚪 API Gateway :8080]
    
    AG --> US[👤 User Service :8082]
    AG --> ProdS[📦 Product Service :8083]
    AG --> OS[📋 Order Service :8084]
    AG --> PS[⚡ Purchase Service :8081]
    
    US --> UserDB[(user-db :5433)]
    ProdS --> ProductDB[(product-db :5434)]
    OS --> OrderDB[(order-db :5435)]
    PayS[💳 Payment Service :8086] --> PaymentDB[(payment-db :5436)]
    ProdS --> Redis[(Redis)]
    PS --> Redis
    PS --> RMQ[RabbitMQ]
    RMQ --> OW[👷 Order Worker]
    OW --> OS
    OW --> ProdS
    OW --> PayS
```

## 🛠️ Core Technologies

| Technology | Purpose |
| :--- | :--- |
| **Go 1.23** | Fast, concurrent, simple syntax |
| **Gin** | Lightweight HTTP framework |
| **Redis** | In-memory stock & idempotency |
| **RabbitMQ** | Async message processing |
| **PostgreSQL** | Persistent storage (3 separate databases) |
| **Docker Compose** | Local orchestration |


## 🏁 Quick Start

### 🐳 With Docker Compose

```bash
# 1. Setup Environment Variables
cp .env.example .env

# 2. Start all services (6 services + 3 databases)
cd flashsale && docker-compose up --build

# 3. Seed the databases (In a new terminal)
# First, update .env for local seeding:
# USER_POSTGRES_URL=postgres://postgres:postgres@localhost:5433/flashsale_users?sslmode=disable
# PRODUCT_POSTGRES_URL=postgres://postgres:postgres@localhost:5434/flashsale_products?sslmode=disable
cd scripts && go run seed.go
```

## 🧪 API Endpoints & Testing Guide

All endpoints go through the **API Gateway** at `http://localhost:8080`.

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| POST | `/auth/signup` | Create new user | No |
| POST | `/auth/login` | Get JWT token | No |
| GET | `/products` | List all products | No |
| GET | `/products/:id` | Get product details | No |
| GET | `/products/:id/stock` | Get current stock | No |
| POST | `/purchase` | Make a purchase | Yes |
| GET | `/orders/:id` | Get order by ID | Yes |
| GET | `/my-orders` | Get user's orders | Yes |
| DELETE | `/orders/:id` | Cancel order | Yes |

---

### Testing Commands (PowerShell)

### 1\. 📝 Sign Up

```powershell
$response = curl.exe -s -X POST http://localhost:8080/auth/signup `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "password123"}'
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": true,
  "data": {
    "user_id": "200a19e1-4a0e-4086-8509-0a9f5fd40453"
  },
  "message": "Account created successfully. You can now log in."
}
```

---

### 2\. 🔑 Login

```powershell
$response = curl.exe -s -X POST "http://localhost:8080/auth/login" `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "password123"}'

$json = $response | ConvertFrom-Json
$TOKEN = $json.data.token
$json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": true,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 86400,
    "user_id": "200a19e1-4a0e-4086-8509-0a9f5fd40453",
    "email": "tester@example.com",
    "role": "tester"
  },
  "message": "Login successful. Welcome back!"
}
```

---

### 3\. 📦 List Products

```powershell
$response = curl.exe -s http://localhost:8080/products
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "products": [
    {
      "id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc",
      "name": "iPhone 15 Pro",
      "description": "Latest flagship smartphone with A17 Pro chip. Available in Blue Titanium, Black Titanium. Storage: 128GB, 256GB, 512GB.",
      "price": 999,
      "stock": 100
    }
  ]
}
```

---

### 4\. 📊 Get Product Stock

```powershell
$response = curl.exe -s http://localhost:8080/products/9b633b6b-4384-42ea-9675-2c3788f8e4bc/stock
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc",
  "stock": 100
}
```

---

### 5\. 🛍️ Purchase Item

```powershell
# Add "notes" for variants and "payment_method" for payment simulation
# Payment methods: mock_success (default), mock_fail, mock_pending
$response = curl.exe -s -X POST "http://localhost:8080/purchase" `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc", "qty": 1, "notes": "Color: Blue", "payment_method": "mock_success"}'

$json = $response | ConvertFrom-Json
$ORDER_ID = $json.data.order_id
echo "Order Created: $ORDER_ID"
$json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": true,
  "data": {
    "order_id": "e4858bd8-5542-4d54-9fe1-de5ce07be721",
    "product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc",
    "product_name": "iPhone 15 Pro",
    "qty": 1,
    "status": "PENDING"
  },
  "message": "Your order for iPhone 15 Pro has been placed and is being processed."
}
```

---

### 6\. 🔍 Check Order Status

```powershell
$response = curl.exe -s -X GET http://localhost:8080/orders/$ORDER_ID `
  -H "Authorization: Bearer $TOKEN"
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": true,
  "data": {
    "order_id": "e4858bd8-5542-4d54-9fe1-de5ce07be721",
    "status": "SUCCESS"
  },
  "message": "Order retrieved successfully."
}
```

---

### 7\. 📋 Get My Orders

```powershell
$response = curl.exe -s http://localhost:8080/my-orders `
  -H "Authorization: Bearer $TOKEN"
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "orders": [
    {
      "id": "e4858bd8-5542-4d54-9fe1-de5ce07be721",
      "user_id": "200a19e1-4a0e-4086-8509-0a9f5fd40453",
      "product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc",
      "product_name": "iPhone 15 Pro",
      "product_price": 999,
      "qty": 1,
      "notes": "Color: Blue Titanium, Storage: 256GB",
      "status": "SUCCESS",
      "created_at": "2025-12-24T08:47:39.164363Z"
    }
  ]
}
```

---

### 8\. 🚫 Cancel Order

```powershell
$response = curl.exe -s -X DELETE http://localhost:8080/orders/$ORDER_ID `
  -H "Authorization: Bearer $TOKEN"
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": true,
  "data": {
    "order_id": "e4858bd8-5542-4d54-9fe1-de5ce07be721",
    "product_name": "iPhone 15 Pro"
  },
  "message": "Your order has been cancelled successfully."
}
```

---

## ⚠️ Failure Test Cases

### Test Case A: 🔁 Duplicate Purchase (Idempotency)

```powershell
# Try buying the same product again immediately (within 2 minutes)
$response = curl.exe -s -X POST "http://localhost:8080/purchase" `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc", "qty": 1}'
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": false,
  "data": {
    "product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc"
  },
  "message": "You already have a pending order for iPhone 15 Pro. Please wait for it to complete.",
  "error": "DUPLICATE_PURCHASE"
}
```

---

### Test Case B: 📉 Out of Stock

```powershell
# Try buying more than available stock (MacBook has only 8 units)
$response = curl.exe -s -X POST http://localhost:8080/purchase `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": "2c287f58-e30f-4eeb-91d8-4a8a4dfe2106", "qty": 10}'
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": false,
  "data": {
    "product_id": "2c287f58-e30f-4eeb-91d8-4a8a4dfe2106"
  },
  "message": "Sorry, MacBook Air M3 is currently out of stock.",
  "error": "OUT_OF_STOCK"
}
```

---

### Test Case C: ⛔ Unauthorized

```powershell
# Try accessing without a token
$response = curl.exe -s -X POST http://localhost:8080/purchase `
  -H "Content-Type: application/json" `
  -d '{"product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc", "qty": 1}'
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": false,
  "data": null,
  "message": "You need to be logged in to access this resource.",
  "error": "MISSING_AUTH_HEADER"
}
```

---

### Test Case D: ❌ Invalid Credentials

```powershell
$response = curl.exe -s -X POST http://localhost:8080/auth/login `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "wrongpassword"}'
$response | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Response:**
```json
{
  "success": false,
  "data": null,
  "message": "Invalid email or password. Please try again.",
  "error": "INVALID_CREDENTIALS"
}
```

---

### Test Case E: ⏳ Rate Limiting (120 requests/minute)

```powershell
# Spam the API to trigger rate limiting
for ($i = 1; $i -le 150; $i++) {
  $response = curl.exe -s http://localhost:8080/products
  if ($response -match "Rate limit") {
    Write-Host "Request $i : Rate limited!"
    Write-Host $response
    break
  }
}
```

**Response (after 120 requests):**
```json
{"error": "Too many requests. Please try again later.", "message": "Rate limit exceeded"}
```

---

### Test Case F: 💳 Payment Failure

```powershell
# Step 1: Create order with mock_fail payment method
$response = curl.exe -s -X POST "http://localhost:8080/purchase" `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": "c452cc7b-4e6f-44e8-9f18-fe74ba3a6b6e", "qty": 1, "payment_method": "mock_fail"}'
$json = $response | ConvertFrom-Json
$FAIL_ORDER_ID = $json.data.order_id
echo "Order Created: $FAIL_ORDER_ID (status: PENDING)"
$json | ConvertTo-Json -Depth 5

# Step 2: Wait for async payment processing (2 seconds)
Start-Sleep -Seconds 2

# Step 3: Check order status - should be FAILED
$status = curl.exe -s -X GET "http://localhost:8080/orders/$FAIL_ORDER_ID" `
  -H "Authorization: Bearer $TOKEN"
$status | ConvertFrom-Json | ConvertTo-Json -Depth 5
```

**Step 1 Response (Immediate - Order Created):**
```json
{
  "success": true,
  "data": {
    "order_id": "...",
    "product_name": "AirPods Pro",
    "status": "PENDING"
  },
  "message": "Your order for AirPods Pro has been placed and is being processed."
}
```

**Step 3 Response (After 2 seconds - Payment Failed):**
```json
{
  "success": true,
  "data": {
    "order_id": "...",
    "status": "FAILED"
  },
  "message": "Order retrieved successfully."
}
```

> **Note:** Payment processing is **asynchronous**. The initial response shows `PENDING`, then the order-worker processes the payment in the background. Check `GET /orders/:id` to see the final status.

---

## 💾 Database Schema

The system uses **4 separate PostgreSQL databases** for strict microservices isolation:

### user-db (port 5433)
```sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'user',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### product-db (port 5434)
```sql
CREATE TABLE products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price INT NOT NULL,
    stock INT NOT NULL CHECK (stock >= 0)
);
```

### order-db (port 5435)
```sql
CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    product_id UUID NOT NULL,
    product_name VARCHAR(255) NOT NULL,
    product_price INT NOT NULL,
    qty INT NOT NULL,
    notes TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

### payment-db (port 5436)
```sql
CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    amount INT NOT NULL,
    method VARCHAR(50) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

## 📄 License

MIT - Use for portfolio or personal projects!