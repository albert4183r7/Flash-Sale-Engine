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
├── 👷 order-worker/         # Async order processing (RabbitMQ Consumer)
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
6.  **👷 Order Worker:** Async background worker that processes orders.

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
    ProdS --> Redis[(Redis)]
    PS --> Redis
    PS --> RMQ[RabbitMQ]
    RMQ --> OW[👷 Order Worker]
    OW --> OS
    OW --> ProdS
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

> **API Version**: v1 - All endpoints are prefixed with `/api/v1`

| Method | Endpoint | Description | Auth |
|--------|----------|-------------|------|
| GET | `/health` | Health check | No |
| POST | `/api/v1/auth/signup` | Create new user | No |
| POST | `/api/v1/auth/login` | Get JWT token | No |
| GET | `/api/v1/products` | List all products | No |
| GET | `/api/v1/products/:id` | Get product details | No |
| GET | `/api/v1/products/:id/stock` | Get current stock | No |
| POST | `/api/v1/users/:user_id/orders` | Create order (purchase) | Yes |
| GET | `/api/v1/users/:user_id/orders` | Get user's orders | Yes |
| GET | `/api/v1/users/:user_id/orders/:order_id` | Get order by ID | Yes |
| DELETE | `/api/v1/users/:user_id/orders/:order_id` | Cancel order | Yes |

---

### Testing Commands (PowerShell)

### 1\. 📝 Sign Up

```powershell
$response = curl.exe -s -X POST http://localhost:8080/api/v1/auth/signup `
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
$response = curl.exe -s -X POST "http://localhost:8080/api/v1/auth/login" `
  -H "Content-Type: application/json" `
  -d '{"email": "tester@example.com", "password": "password123"}'

$json = $response | ConvertFrom-Json
$TOKEN = $json.data.token
$USER_ID = $json.data.user_id
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
$response = curl.exe -s http://localhost:8080/api/v1/products
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
$response = curl.exe -s http://localhost:8080/api/v1/products/9b633b6b-4384-42ea-9675-2c3788f8e4bc/stock
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

### 5\. 🛍️ Create Order (Purchase)

```powershell
# Add "notes" for order variants
# Note: USER_ID in URL must match your JWT token
$response = curl.exe -s -X POST "http://localhost:8080/api/v1/users/$USER_ID/orders" `
  -H "Authorization: Bearer $TOKEN" `
  -H "Content-Type: application/json" `
  -d '{"product_id": "9b633b6b-4384-42ea-9675-2c3788f8e4bc", "qty": 1, "notes": "Color: Blue"}'

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
$response = curl.exe -s -X GET "http://localhost:8080/api/v1/users/$USER_ID/orders/$ORDER_ID" `
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

### 7\. 📋 Get User Orders

```powershell
$response = curl.exe -s "http://localhost:8080/api/v1/users/$USER_ID/orders" `
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
$response = curl.exe -s -X DELETE "http://localhost:8080/api/v1/users/$USER_ID/orders/$ORDER_ID" `
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
$response = curl.exe -s -X POST "http://localhost:8080/api/v1/users/$USER_ID/orders" `
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
$response = curl.exe -s -X POST "http://localhost:8080/api/v1/users/$USER_ID/orders" `
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
# Try accessing without a token (replace with any valid UUID)
$response = curl.exe -s -X POST "http://localhost:8080/api/v1/users/550e8400-e29b-41d4-a716-446655440000/orders" `
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
$response = curl.exe -s -X POST http://localhost:8080/api/v1/auth/login `
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
  $response = curl.exe -s http://localhost:8080/api/v1/products
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

## 💾 Database Schema

The system uses **3 separate PostgreSQL databases** for strict microservices isolation:

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

## ☁️ GCP GKE Autopilot Deployment

This project is designed for **GKE Autopilot** in the Jakarta region (`asia-southeast2`) for optimal performance and Indonesian data residency compliance.

### Prerequisites
- Google Cloud SDK installed (`gcloud`)
- `kubectl` installed
- Terraform >= 1.5 installed
- GCP project with billing enabled (Free Trial $300 works!)

### Quick Deploy (6 Services)

```bash
# 1. Enable required APIs
gcloud services enable container.googleapis.com sqladmin.googleapis.com \
  redis.googleapis.com pubsub.googleapis.com secretmanager.googleapis.com \
  artifactregistry.googleapis.com cloudbuild.googleapis.com servicenetworking.googleapis.com

# 2. Deploy infrastructure with Terraform
cd terraform
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your project ID and secrets
terraform init && terraform apply

# 3. Get cluster credentials
gcloud container clusters get-credentials flashsale-cluster \
  --region asia-southeast2 --project YOUR_PROJECT_ID

# 4. Create secrets
kubectl apply -f k8s/namespace.yaml
kubectl create secret generic flashsale-secrets --namespace flashsale \
  --from-literal=jwt-secret=YOUR_JWT_SECRET \
  --from-literal=db-password=YOUR_DB_PASSWORD

# 5. Build and push images
export PROJECT_ID=your-project-id REGION=asia-southeast2
gcloud auth configure-docker ${REGION}-docker.pkg.dev
cd flashsale
for svc in api-gateway user-service product-service order-service purchase-service order-worker; do
  docker build -f deployments/Dockerfile.$svc -t ${REGION}-docker.pkg.dev/${PROJECT_ID}/flashsale/${svc}:latest .
  docker push ${REGION}-docker.pkg.dev/${PROJECT_ID}/flashsale/${svc}:latest
done

# 6. Deploy to GKE
cd ../k8s
kubectl apply -f namespace.yaml -f service-account.yaml -f configmap.yaml -f services/
```

### Production Infrastructure
| Component | GCP Service | Configuration |
|-----------|-------------|---------------|
| Compute | GKE Autopilot | Zero cold starts, auto-scaling |
| Databases | Cloud SQL (PostgreSQL 17) | 3 HA databases |
| Cache | Memorystore (Redis 7) | HA with replica |
| Messaging | Cloud Pub/Sub | Replaces RabbitMQ |
| Secrets | Secret Manager | JWT & DB credentials |
| Container Registry | Artifact Registry | Private Docker images |

> 📖 See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) for detailed deployment guide and [flashsale/docs/architecture.md](flashsale/docs/architecture.md) for architecture diagrams.

---

## �📄 License

MIT - Use for portfolio or personal projects!
