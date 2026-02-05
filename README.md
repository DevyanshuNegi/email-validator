# Hybrid Email Validator SaaS

A high-performance, scalable email validation system built with Next.js (Hub) and Go (Worker), using Redis for queue management and PostgreSQL for data persistence.

## 📋 Table of Contents

- [Architecture](#architecture)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Prerequisites](#prerequisites)
- [Getting Started](#getting-started)
- [Running the Application](#running-the-application)
- [API Documentation](#api-documentation)
- [Worker Configuration](#worker-configuration)
- [Safe Limits & Best Practices](#safe-limits--best-practices)
- [Local Testing](#local-testing)
- [Troubleshooting](#troubleshooting)
- [Production Considerations](#production-considerations)

## 🏗️ Architecture

### System Overview

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP/JSON
       ▼
┌─────────────────────────────────────────────────┐
│              Next.js Hub (API)                   │
│  - Receives email validation requests           │
│  - Creates Job records in PostgreSQL            │
│  - Pushes emails to Redis queue                 │
│  - Returns job status and results               │
└──────┬──────────────────────┬───────────────────┘
       │                      │
       │ PostgreSQL           │ Redis Queue
       ▼                      ▼
┌──────────────┐      ┌──────────────┐
│  PostgreSQL  │      │    Redis     │
│  - Jobs      │      │  email_queue │
│  - EmailCheck│      └──────┬───────┘
└──────────────┘             │
                              │ BRPOP
                              ▼
                    ┌─────────────────────┐
                    │   Go Worker Pool     │
                    │  (50 Goroutines)     │
                    │  - SMTP Validation  │
                    │  - Updates Database │
                    └─────────────────────┘
```

### Component Details

#### 1. **Hub (Next.js)**
- **Purpose**: API gateway and job management
- **Responsibilities**:
  - Accept email validation requests
  - Create job records in PostgreSQL
  - Queue emails in Redis
  - Provide job status API
- **Location**: `/hub`

#### 2. **Worker (Go)**
- **Purpose**: High-performance email validation
- **Responsibilities**:
  - Process emails from Redis queue
  - Perform SMTP validation (RFC 5321)
  - Update database with results
- **Location**: `/worker`
- **Concurrency**: 50 goroutines by default

#### 3. **Redis**
- **Purpose**: Message queue for async processing
- **Queue Name**: `email_queue`
- **Operation**: BRPOP (blocking right pop)

#### 4. **PostgreSQL**
- **Purpose**: Data persistence
- **Tables**:
  - `Job`: Job metadata
  - `EmailCheck`: Individual email validation results

### Data Flow

1. **Request Phase**:
   ```
   Client → POST /api/verify → Hub creates Job → Emails queued in Redis
   ```

2. **Processing Phase**:
   ```
   Worker BRPOP → SMTP Check → Update EmailCheck table
   ```

3. **Retrieval Phase**:
   ```
   Client → GET /api/job/[id] → Hub returns results
   ```

## 🛠️ Tech Stack

### Hub
- **Framework**: Next.js 14 (App Router)
- **Language**: TypeScript
- **ORM**: Prisma 7
- **Database Driver**: PostgreSQL (via `@prisma/adapter-pg`)
- **Queue Client**: ioredis
- **Runtime**: Node.js

### Worker
- **Language**: Go 1.21+
- **Redis Client**: `github.com/redis/go-redis/v9`
- **PostgreSQL Driver**: `github.com/lib/pq`
- **SMTP**: Standard library `net` package

### Infrastructure
- **Database**: PostgreSQL 16 (Alpine)
- **Queue**: Redis 7 (Alpine)
- **Orchestration**: Docker Compose

## 📁 Project Structure

```
email-validator/
├── hub/                          # Next.js API Hub
│   ├── src/
│   │   ├── app/
│   │   │   └── api/
│   │   │       ├── verify/       # POST /api/verify
│   │   │       └── job/[id]/    # GET /api/job/[id]
│   │   └── lib/
│   │       ├── prisma.ts        # Prisma client
│   │       └── redis.ts         # Redis client
│   ├── prisma/
│   │   ├── schema.prisma        # Database schema
│   │   └── migrations/          # Database migrations
│   └── package.json
│
├── worker/                       # Go Worker
│   ├── main.go                  # Worker orchestration
│   ├── smtp.go                  # SMTP validation logic
│   └── go.mod
│
├── docker-compose.yml           # Infrastructure setup
└── README.md                    # This file
```

## 📦 Prerequisites

### Required Software

- **Node.js**: 18.x or higher
- **Go**: 1.21 or higher
- **Docker**: 20.x or higher
- **Docker Compose**: 2.x or higher

### Verify Installation

```bash
node --version    # Should be v18.x or higher
go version        # Should be go1.21 or higher
docker --version  # Should be 20.x or higher
docker compose version  # Should be 2.x or higher
```

## 🚀 Getting Started

### 1. Clone and Navigate

```bash
cd /home/devyanshu/Desktop/email-validator
```

### 2. Start Infrastructure

```bash
# Start PostgreSQL and Redis
docker compose up -d

# Verify services are running
docker compose ps
```

**Expected Output:**
```
NAME                       STATUS
email-validator-postgres   Up (healthy)
email-validator-redis      Up (healthy)
```

### 3. Setup Hub (Next.js)

```bash
cd hub

# Install dependencies
npm install

# Setup environment
# .env file should already have DATABASE_URL
# DATABASE_URL="postgresql://postgres:postgres@localhost:5433/emailvalidator"

# Run database migrations
npx prisma migrate dev

# Generate Prisma client
npx prisma generate
```

### 4. Setup Worker (Go)

```bash
cd ../worker

# Install dependencies
go mod download

# Build worker
go build -o validator-worker .
```

## ▶️ Running the Application

### Start All Services

#### Terminal 1: Infrastructure (if not already running)
```bash
docker compose up -d
```

#### Terminal 2: Hub (Next.js API)
```bash
cd hub
npm run dev
```

**Expected Output:**
```
▲ Next.js 16.1.6
- Local:        http://localhost:8080
```

#### Terminal 3: Worker (Go)
```bash
cd worker
./validator-worker
```

**Expected Output:**
```
🚀 Starting Email Validator Worker...
✅ Connected to Redis
✅ Connected to PostgreSQL
✅ Started 50 workers
📬 Listening for emails in queue: email_queue
```

### Verify Everything is Running

```bash
# Check services
docker compose ps

# Test API
curl http://localhost:8080

# :LEARN
# Test Redis
docker exec email-validator-redis redis-cli PING
# Should return: PONG

# Test PostgreSQL
PGPASSWORD=postgres psql -h localhost -p 5433 -U postgres -d emailvalidator -c "SELECT COUNT(*) FROM \"Job\";"
```

## 📚 API Documentation

### Base URL
```
http://localhost:8080
```

### Endpoints

#### 1. Create Validation Job

**POST** `/api/verify`

Creates a new email validation job and queues emails for processing.

**Request Body:**
```json
["email1@example.com", "email2@example.com", "email3@example.com"]
```

**Response:** `201 Created`
```json
{
  "jobId": "uuid-here",
  "totalEmails": 3,
  "status": "PENDING"
}
```

**Example:**
```bash
curl -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["test@example.com", "user@domain.com"]'
```

**Error Responses:**
- `400 Bad Request`: Invalid input (empty array, non-array, etc.)
- `500 Internal Server Error`: Server error

---

#### 2. Get Job Status

**GET** `/api/job/[id]`

Retrieves the status and results of a validation job.

**Response:** `200 OK`
```json
{
  "id": "uuid-here",
  "totalEmails": 3,
  "status": "PENDING",
  "createdAt": "2026-02-03T06:17:34.901Z",
  "emailChecks": [
    {
      "id": "uuid-here",
      "email": "test@example.com",
      "status": "VALID",
      "smtpCode": 250,
      "bounceReason": null
    },
    {
      "id": "uuid-here",
      "email": "invalid@fake.com",
      "status": "INVALID",
      "smtpCode": 550,
      "bounceReason": "No MX records found"
    }
  ]
}
```

**Example:**
```bash
curl http://localhost:8080/api/job/your-job-id-here
```

**Error Responses:**
- `404 Not Found`: Job not found
- `500 Internal Server Error`: Server error

---

### Status Values

| Status | Description | SMTP Code Range |
|--------|-------------|-----------------|
| `PENDING` | Not yet processed | - |
| `VALID` | Email is valid | 250 |
| `INVALID` | Email is invalid | 550, 551, 553 |
| `GREYLISTED` | Temporarily rejected | 450, 451 |
| `CATCH_ALL` | Catch-all domain | 251, 252 |
| `UNKNOWN` | Connection/timeout error | 0 |

## ⚙️ Worker Configuration

### Environment Variables

The worker uses hardcoded defaults (can be modified in `main.go`):

```go
// Redis Configuration
Addr:     "localhost:6379"
Password: ""  // no password
DB:       0   // default DB

// PostgreSQL Configuration
Connection: "postgres://postgres:postgres@localhost:5433/emailvalidator?sslmode=disable"

// Worker Pool
Worker Count: 50 goroutines
```

### SMTP Configuration

Located in `worker/smtp.go`:

- **Connection Timeout**: 5 seconds
- **Read/Write Timeout**: 10 seconds
- **Port**: 25 (SMTP)
- **HELO Domain**: `yourdomain.com` (configurable)

### Adjusting Worker Count

Edit `worker/main.go`:

```go
const (
    workerCount = 50  // Change this value
    redisQueue  = "email_queue"
)
```

Rebuild:
```bash
cd worker
go build -o validator-worker .
```

## 🛡️ Safe Limits & Best Practices

### Rate Limiting

#### Recommended Limits

| Component | Limit | Reason |
|-----------|-------|--------|
| **API Requests** | 100 req/min | Prevent queue overflow |
| **Emails per Request** | 100 emails | Balance throughput |
| **Concurrent Workers** | 50 goroutines | Optimal for most systems |
| **SMTP Timeout** | 5 seconds | Prevent hanging connections |
| **Redis Queue Size** | Monitor < 10,000 | Prevent memory issues |

### API Rate Limiting (Recommended)

Add rate limiting to Next.js API routes:

```typescript
// Example: Add to /hub/src/app/api/verify/route.ts
// Use a library like 'rate-limiter-flexible' or similar
```

### Database Limits

- **Connection Pool**: Configure Prisma connection pool
- **Query Timeout**: Set appropriate timeouts
- **Indexes**: Already created on `jobId` for EmailCheck

### Redis Limits

- **Memory**: Monitor Redis memory usage
- **Queue Length**: Alert if queue > 10,000 items
- **Connection Pool**: Worker uses single connection (sufficient for BRPOP)

### SMTP Best Practices

1. **Respect Rate Limits**: Many mail servers limit connections
2. **Handle Greylisting**: Retry logic for 450/451 responses
3. **Timeout Handling**: 5-second timeout prevents hanging
4. **Connection Reuse**: Not applicable (one-time checks)

### Worker Scaling

**Horizontal Scaling:**
- Run multiple worker instances
- Each worker processes from the same Redis queue
- Workers coordinate via Redis BRPOP (atomic operation)

**Vertical Scaling:**
- Increase worker goroutines (50 → 100)
- Monitor system resources (CPU, memory, network)

### Monitoring Recommendations

1. **Queue Length**: `redis-cli LLEN email_queue`
2. **Worker Status**: Check worker process/logs
3. **Database Size**: Monitor PostgreSQL growth
4. **API Response Times**: Monitor Next.js performance
5. **SMTP Success Rate**: Track VALID vs INVALID vs UNKNOWN

## 🧪 Local Testing

### Quick Test

```bash
# 1. Create a job
JOB_ID=$(curl -s -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["test@example.com", "invalid@fake-domain-xyz.com"]' \
  | python3 -c "import sys, json; print(json.load(sys.stdin)['jobId'])")

echo "Job ID: $JOB_ID"

# 2. Wait for processing (5-10 seconds)
sleep 5

# 3. Check results
curl http://localhost:8080/api/job/$JOB_ID | python3 -m json.tool
```

### Comprehensive Test Suite

```bash
#!/bin/bash
# test-suite.sh

echo "=== Testing Email Validator ==="

# Test 1: Create job
echo "1. Creating job..."
RESPONSE=$(curl -s -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["test1@example.com", "test2@nonexistent-xyz-999.com", "admin@github.com"]')
JOB_ID=$(echo $RESPONSE | python3 -c "import sys, json; print(json.load(sys.stdin)['jobId'])")
echo "✅ Job created: $JOB_ID"

# Test 2: Check queue
echo "2. Checking Redis queue..."
QUEUE_LEN=$(docker exec email-validator-redis redis-cli LLEN email_queue)
echo "✅ Queue length: $QUEUE_LEN"

# Test 3: Wait for processing
echo "3. Waiting for worker to process (10 seconds)..."
sleep 10

# Test 4: Verify results
echo "4. Checking results..."
curl -s http://localhost:8080/api/job/$JOB_ID | python3 -m json.tool

# Test 5: Error handling
echo "5. Testing error handling..."
curl -s -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '[]' | python3 -m json.tool

echo "=== Tests Complete ==="
```

### Testing SMTP Validation

**Valid Email Test:**
```bash
# Use a known valid email (may timeout due to port 25 blocking)
curl -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["support@microsoft.com"]'
```

**Invalid Email Test:**
```bash
# Use a clearly invalid domain
curl -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["test@nonexistent-domain-xyz-12345.com"]'
```

### Database Testing

```bash
# Check job count
PGPASSWORD=postgres psql -h localhost -p 5433 -U postgres -d emailvalidator \
  -c "SELECT COUNT(*) FROM \"Job\";"

# Check email check results
PGPASSWORD=postgres psql -h localhost -p 5433 -U postgres -d emailvalidator \
  -c "SELECT email, status, \"smtpCode\" FROM \"EmailCheck\" ORDER BY \"createdAt\" DESC LIMIT 10;"
```

### Redis Testing

```bash
# Check queue length
docker exec email-validator-redis redis-cli LLEN email_queue

# View queue contents (without removing)
docker exec email-validator-redis redis-cli LRANGE email_queue 0 10

# Monitor queue in real-time
watch -n 1 'docker exec email-validator-redis redis-cli LLEN email_queue'
```

### Worker Testing

```bash
# Run worker with timeout to see output
cd worker
timeout 15 ./validator-worker

# Check worker is processing
# Look for output like:
# [Worker 1] 🔍 Checking: test@example.com
# [Worker 1] ✅ VALID: test@example.com (Code: 250)
```

## 🔧 Troubleshooting

### Common Issues

#### 1. PostgreSQL Connection Failed

**Error:**
```
Failed to connect to PostgreSQL: connection refused
```

**Solution:**
```bash
# Check if PostgreSQL is running
docker compose ps postgres

# If not running, start it
docker compose up -d postgres

# Check port (should be 5433)
docker compose ps
```

#### 2. Redis Connection Failed

**Error:**
```
Failed to connect to Redis: dial tcp :6379: connect: connection refused
```

**Solution:**
```bash
# Check if Redis is running
docker compose ps redis

# Start Redis
docker compose up -d redis

# Test connection
docker exec email-validator-redis redis-cli PING
```

#### 3. Port 5432 Already in Use

**Error:**
```
ERROR: address already in use: 5432
```

**Solution:**
- The docker-compose.yml uses port 5433 to avoid conflicts
- Update DATABASE_URL in `.env` to use port 5433

#### 4. Prisma Migration Errors

**Error:**
```
Error: Migration failed
```

**Solution:**
```bash
cd hub
# Reset database (WARNING: deletes all data)
npx prisma migrate reset

# Or create a new migration
npx prisma migrate dev --name fix_migration
```

#### 5. Worker Not Processing Emails

**Symptoms:**
- Queue has items but worker shows no activity

**Solution:**
```bash
# Check worker is running
ps aux | grep validator-worker

# Check Redis queue
docker exec email-validator-redis redis-cli LLEN email_queue

# Restart worker
pkill -f validator-worker
cd worker
./validator-worker
```

#### 6. SMTP Timeouts (UNKNOWN Status)

**Expected Behavior:**
Many email providers block port 25 connections to prevent spam. This results in:
- Connection timeouts
- Connection refused errors
- UNKNOWN status with Code 0

**This is normal** and indicates the system is working correctly. The worker handles these gracefully.

### Debug Mode

**Hub (Next.js):**
```bash
cd hub
# Already in dev mode with logging
npm run dev
```

**Worker (Go):**
Add more logging in `worker/main.go`:
```go
log.Printf("[DEBUG] Processing job: %+v", job)
```

## 🚢 Production Considerations

### Security

1. **Environment Variables**: Use `.env` files, never commit secrets
2. **Database Credentials**: Use strong passwords
3. **Redis Authentication**: Enable Redis password
4. **API Rate Limiting**: Implement rate limiting
5. **Input Validation**: Already implemented in API

### Performance

1. **Connection Pooling**: Configure Prisma connection pool
2. **Worker Scaling**: Run multiple worker instances
3. **Database Indexing**: Already implemented
4. **Redis Persistence**: Configure Redis persistence
5. **Monitoring**: Add monitoring (Prometheus, Grafana)

### Deployment

1. **Docker**: Build production Docker images
2. **Kubernetes**: Deploy with K8s for scaling
3. **Load Balancer**: Use load balancer for Hub
4. **Database**: Use managed PostgreSQL (AWS RDS, etc.)
5. **Redis**: Use managed Redis (AWS ElastiCache, etc.)

### Environment Variables (Production)

**Hub (.env):**
```env
DATABASE_URL="postgresql://user:password@host:5432/dbname?sslmode=require"
REDIS_URL="redis://:password@host:6379"
NODE_ENV="production"
```

**Worker (Environment or Config):**
```bash
export REDIS_ADDR="host:6379"
export REDIS_PASSWORD="password"
export DATABASE_URL="postgresql://user:password@host:5432/dbname?sslmode=require"
```

## 📊 Monitoring & Metrics

### Key Metrics to Monitor

1. **Queue Length**: `redis-cli LLEN email_queue`
2. **Processing Rate**: Emails processed per minute
3. **Success Rate**: VALID vs INVALID vs UNKNOWN
4. **API Response Time**: P95, P99 latencies
5. **Database Size**: Job and EmailCheck table growth
6. **Worker Health**: Process uptime, error rate

### Logging

**Hub Logs:**
- Check Next.js dev server output
- API errors logged to console

**Worker Logs:**
- Real-time output with emoji indicators
- Errors logged with context

**Database Logs:**
```bash
docker compose logs postgres
```

**Redis Logs:**
```bash
docker compose logs redis
```

## 📝 License

This project is part of a Hybrid Email Validator SaaS system.

## 🤝 Contributing

1. Follow the architecture patterns
2. Maintain code quality
3. Add tests for new features
4. Update documentation

---

**Last Updated**: February 2026
**Version**: 1.0.0
