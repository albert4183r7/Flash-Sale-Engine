# Flash Sale System

A professional-grade flash sale backend engine built with Go microservices architecture. This project demonstrates distributed systems concepts including:
- Microservices communication
- JWT authentication
- Redis-based stock management and idempotency
- Async message processing with RabbitMQ
- PostgreSQL persistence

## Architecture
The system consists of 3 microservices:
1. **API Gateway** (port 5000/8080) - Entry point with JWT auth, rate limiting, routing
2. **Purchase Service** (port 8081) - Stock validation, idempotency, event publishing
3. **Order Worker** - Async order processing from RabbitMQ to PostgreSQL

## Project Structure
```
flashsale/
├── api-gateway/          # Gin-based HTTP gateway
├── purchase-service/     # Purchase logic with Redis
├── order-worker/         # RabbitMQ consumer + PostgreSQL
├── common/               # Shared utilities
├── deployments/          # Docker configurations
├── docker-compose.yml    # Full stack orchestration
└── README.md             # Complete documentation
```

## System Architecture

```mermaid
graph TB
    subgraph "Client Layer"
        C[Client/Browser]
    end
    
    subgraph "API Layer"
        AG[API Gateway<br/>Port 8080]
    end
    
    subgraph "Service Layer"
        PS[Purchase Service<br/>Port 8081]
    end
    
    subgraph "Worker Layer"
        OW[Order Worker]
    end
    
    subgraph "Data Layer"
        R[(Redis<br/>Stock + Idempotency)]
        RMQ[(RabbitMQ<br/>Message Queue)]
        PG[(PostgreSQL<br/>Order Storage)]
    end
    
    C -->|HTTP Request| AG
    AG -->|JWT Validation| AG
    AG -->|Forward Request| PS
    PS -->|SETNX Idempotency| R
    PS -->|DECR Stock| R
    PS -->|Publish Event| RMQ
    RMQ -->|Consume| OW
    OW -->|Store Order| PG
    
    style AG fill:#e1f5fe
    style PS fill:#fff3e0
    style OW fill:#e8f5e9
    style R fill:#ffebee
    style RMQ fill:#f3e5f5
    style PG fill:#e0f2f1
```

## Flash Sale Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant AG as API Gateway
    participant PS as Purchase Service
    participant R as Redis
    participant RMQ as RabbitMQ
    participant OW as Order Worker
    participant PG as PostgreSQL
    
    C->>AG: POST /purchase (JWT + Product + Qty)
    AG->>AG: Validate JWT Token
    AG->>PS: Forward Purchase Request
    
    PS->>R: SETNX purchase:user:product (TTL 120s)
    alt Key Already Exists
        R-->>PS: false (duplicate)
        PS-->>AG: 400 Duplicate Request
        AG-->>C: Error: Duplicate Purchase
    else Key Set Successfully
        R-->>PS: true (first request)
        PS->>R: DECR product_stock:product
        alt Stock < 0
            R-->>PS: -1 (out of stock)
            PS-->>AG: 400 Out of Stock
            AG-->>C: Error: Out of Stock
        else Stock >= 0
            R-->>PS: new_stock
            PS->>RMQ: Publish order.created
            PS-->>AG: 202 Accepted
            AG-->>C: Order Accepted (OrderID)
            
            RMQ->>OW: Consume Message
            OW->>PG: INSERT order (PENDING)
            OW->>PG: UPDATE status = SUCCESS
        end
    end
```

## Core Technologies

| Technology | Purpose |
|------------|---------|
| Go 1.23 | Fast, concurrent, simple syntax |
| Gin | Lightweight HTTP framework |
| Redis | In-memory stock & idempotency |
| RabbitMQ | Async message processing |
| PostgreSQL | Persistent order storage |
| Docker Compose | Local orchestration |

## Quick Start

### Option 1: With Docker Compose (Full Functionality)

```bash
cd flashsale && docker-compose up --build
# API Gateway: http://localhost:8080
# RabbitMQ Management: http://localhost:15672 (guest/guest)
```

### Option 2: Without Docker (Windows)

#### Prerequisites

**Windows:**
- Go: https://go.dev/dl/
- Redis: https://github.com/tporadowski/redis/releases
- RabbitMQ: https://www.rabbitmq.com/download.html
- PostgreSQL: https://www.postgresql.org/download/windows/

#### Setup

**Windows (PowerShell):**
```powershell
# Start services (from Services panel or):
net start redis
net start rabbitmq
net start postgresql-x64-15

# Create database
psql -U postgres -c "CREATE DATABASE flashsale;"
```

#### Run Services

Open 3 terminals:

**Terminal 1 - API Gateway (PowerShell):**
```powershell
cd flashsale\api-gateway; $env:PORT="5000"; go run main.go
```

**Terminal 2 - Purchase Service (PowerShell):**
```powershell
cd flashsale\purchase-service; $env:PORT="8081"; $env:REDIS_ADDR="localhost:6379"; $env:RABBITMQ_URL="amqp://guest:guest@localhost:5672/"; go run main.go
```

**Terminal 3 - Order Worker (PowerShell):**
```powershell
cd flashsale\order-worker; $env:RABBITMQ_URL="amqp://guest:guest@localhost:5672/"; $env:POSTGRES_URL="postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable"; go run main.go
```

#### Test API

**Windows (PowerShell):**

Set `$PORT` based on environment:
- **Local (Without Docker)**: `$PORT = 5000`
- **Docker (With Docker Compose)**: `$PORT = 8080`

```powershell
$PORT = 5000  # Change to 8080 for Docker

# 1. Authentication - Login
$loginBody = @{ email = "user@example.com"; password = "password123" } | ConvertTo-Json
$loginResponse = Invoke-RestMethod -Uri "http://localhost:$PORT/auth/login" -Method Post -Body $loginBody -ContentType "application/json"
$TOKEN = $loginResponse.data.token
Write-Host "Token: $TOKEN"

# 2. Purchasing - Make a purchase
$purchaseBody = @{ product_id = 1; qty = 1 } | ConvertTo-Json
$purchaseResponse = Invoke-RestMethod -Uri "http://localhost:$PORT/purchase" -Method Post `
  -Body $purchaseBody -ContentType "application/json" `
  -Headers @{ Authorization = "Bearer $TOKEN" }
$ORDER_ID = $purchaseResponse.data.order_id
Write-Host "Order ID: $ORDER_ID"

# 3. Check Order Status
$orderStatus = Invoke-RestMethod -Uri "http://localhost:$PORT/orders/$ORDER_ID" `
  -Headers @{ Authorization = "Bearer $TOKEN" }
$orderStatus | ConvertTo-Json
```

**Terminal Output:**
```json
# Step 1: Login Response
Token: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE3MzM5NDY1NTAsInVzZXJfaWQiOjEsImVtYWlsIjoidXNlckBleGFtcGxlLmNvbSIsInJvbGUiOiJ1c2VyIn0.rN2x5jK8pQrS3tUvWxY2zAb4cDeFgHiJkLmNoPqRsTu

# Step 2: Purchase Response & Order ID
Order ID: 550e8400-e29b-41d4-a716-446655440000

# Step 3: Order Status Response
{
  "success": true,
  "message": "Order status retrieved",
  "data": {
    "order_id": "550e8400-e29b-41d4-a716-446655440000",
    "user_id": 1,
    "product_id": 1,
    "qty": 1,
    "status": "SUCCESS",
    "created_at": "2025-12-01T10:30:45Z"
  }
}
```

## Test Failure Cases

### Test Case 1: Duplicate Purchase (Idempotency Check)

**Windows (PowerShell):**
```powershell
# Try same purchase again
$purchaseBody = @{ product_id = 1; qty = 1 } | ConvertTo-Json
$dupResponse = Invoke-RestMethod -Uri "http://localhost:$PORT/purchase" -Method Post `
  -Body $purchaseBody -ContentType "application/json" `
  -Headers @{ Authorization = "Bearer $TOKEN" }
$dupResponse | ConvertTo-Json
```

**Windows PowerShell Terminal Output:**
```json
{
  "success": false,
  "message": "Duplicate purchase request detected",
  "data": null
}
```

---

### Test Case 2: Out of Stock

**Windows (PowerShell):**
```powershell
$purchaseBody = @{ product_id = 2; qty = 300 } | ConvertTo-Json
$stockResponse = Invoke-RestMethod -Uri "http://localhost:$PORT/purchase" -Method Post `
  -Body $purchaseBody -ContentType "application/json" `
  -Headers @{ Authorization = "Bearer $TOKEN" }
$stockResponse | ConvertTo-Json
```

**Windows PowerShell Terminal Output:**
```json
{
  "success": false,
  "message": "Out of stock",
  "data": {
    "available_stock": 50
  }
}
```

---

### Test Case 3: Unauthorized (Invalid JWT)

**Windows (PowerShell):**
```powershell
$purchaseBody = @{ product_id = 1; qty = 1 } | ConvertTo-Json
$authResponse = Invoke-RestMethod -Uri "http://localhost:$PORT/purchase" -Method Post `
  -Body $purchaseBody -ContentType "application/json" `
  -Headers @{ Authorization = "Bearer invalid-token-here" }
$authResponse | ConvertTo-Json
```

**Windows PowerShell Terminal Output:**
```json
{
  "success": false,
  "message": "Unauthorized: Invalid or expired token",
  "data": null
}
```

---

### Test Case 4: Rate Limiting (429 Too Many Requests)

**Windows (PowerShell):**
```powershell
# Simulate rate limiting with multiple requests
$count = 0
while ($count -lt 101) {
  $productId = Get-Random -Minimum 1 -Maximum 6
  $purchaseBody = @{ product_id = $productId; qty = 1 } | ConvertTo-Json
  try {
    $response = Invoke-RestMethod -Uri "http://localhost:$PORT/purchase" -Method Post `
      -Body $purchaseBody -ContentType "application/json" `
      -Headers @{ Authorization = "Bearer $TOKEN" } -ErrorAction SilentlyContinue
  } catch {}
  $count++
}

# Next request should be rate-limited
$purchaseBody = @{ product_id = 1; qty = 1 } | ConvertTo-Json
Invoke-RestMethod -Uri "http://localhost:$PORT/purchase" -Method Post `
  -Body $purchaseBody -ContentType "application/json" `
  -Headers @{ Authorization = "Bearer $TOKEN" }
```

**Windows PowerShell Terminal Output:**
```json
{
  "success": false,
  "message": "Rate limit exceeded: 100 requests per 60 seconds",
  "data": {
    "retry_after": 15
  }
}
```

---

### Test Case 5: Invalid Credentials (Login Failure)

**Windows (PowerShell):**
```powershell
$loginBody = @{ email = "user@example.com"; password = "wrongpassword" } | ConvertTo-Json
$loginError = Invoke-RestMethod -Uri "http://localhost:$PORT/auth/login" -Method Post `
  -Body $loginBody -ContentType "application/json"
$loginError | ConvertTo-Json
```

**Windows PowerShell Terminal Output:**
```json
{
  "success": false,
  "message": "Invalid email or password",
  "data": null
}
```

## Test Credentials

| Email | Password | Role |
|-------|----------|------|
| user@example.com | password123 | user |
| admin@example.com | admin123 | admin |
| buyer@example.com | buyer123 | user |

## Test Products

| Product ID | Name | Initial Stock |
|------------|------|---------------|
| 1 | iPhone 15 Pro | 100 |
| 2 | MacBook Air M3 | 50 |
| 3 | AirPods Pro | 200 |
| 4 | iPad Pro | 75 |
| 5 | Apple Watch | 150 |

## Key Features

1. **Idempotency**: Redis SETNX with 120s TTL prevents duplicate purchases
2. **Atomic Stock Management**: Lua scripts ensure no overselling under high load
3. **Async Processing**: 202 Accepted response with background RabbitMQ processing
4. **Rate Limiting**: 100 requests/60s per IP
5. **JWT Authentication**: Token-based security with 24-hour expiration


## Environment Variables

| Service | Variable | Default |
|---------|----------|---------|
| API Gateway | PORT | 8080 |
| Purchase | PORT | 8081 |
| Purchase | REDIS_ADDR | localhost:6379 |
| Purchase | RABBITMQ_URL | amqp://guest:guest@localhost:5672/ |
| Worker | RABBITMQ_URL | amqp://guest:guest@localhost:5672/ |
| Worker | POSTGRES_URL | postgres://postgres:postgres@localhost:5432/flashsale?sslmode=disable |

## API Endpoints

### Login
```bash
POST /auth/login
Body: {"email": "user@example.com", "password": "password123"}
```

### Purchase
```bash
POST /purchase (Requires JWT)
Body: {"product_id": 1, "qty": 1}
Response: 202 Accepted
```

### Order Status
```bash
GET /orders/<order-id> (Requires JWT)
```

### Health
```bash
GET /health
```

## Key Concepts

- **Idempotency Key Format**: `purchase:{user_id}:{product_id}`
- **Stock Key Format**: `product_stock:{product_id}`
- **Order Status**: PENDING → SUCCESS/FAILED
- **Rate Limiter**: In-memory, per-IP tracking

## Performance Highlights

- Sub-millisecond Redis stock checks
- No race conditions via atomic Lua scripts
- Immediate API response (202 Accepted)
- Connection pooling for databases
- Graceful shutdown handling

## Error Handling

| Status | Scenario |
|--------|----------|
| 200 | Success |
| 202 | Purchase accepted (async) |
| 400 | Bad request / Out of stock / Duplicate |
| 401 | Unauthorized |
| 429 | Rate limit exceeded |
| 500 | Internal error |
| 503 | Service unavailable |

## Database Schema

```sql
CREATE TABLE orders (
    id UUID PRIMARY KEY,
    user_id INT NOT NULL,
    product_id INT NOT NULL,
    qty INT NOT NULL,
    status VARCHAR(20) DEFAULT 'PENDING',
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX idx_orders_user_id ON orders(user_id);
CREATE INDEX idx_orders_status ON orders(status);
```

## Use Cases

- E-commerce flash sales
- Concert/event ticket releases
- Limited-time meal deals
- Hotel room flash sales
- In-game item releases
- Software license sales

## License

MIT - Use for portfolio or production projects!

---

