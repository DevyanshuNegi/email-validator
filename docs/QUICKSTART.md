# Quick Start Guide

Get the Email Validator running in 5 minutes!

## Prerequisites Check

```bash
node --version    # Need v18+
go version        # Need go1.21+
docker --version  # Need 20.x+
```

## Step-by-Step Setup

### 1. Start Infrastructure (30 seconds)

```bash
cd /home/devyanshu/Desktop/email-validator
docker compose up -d
```

Wait for services to be healthy:
```bash
docker compose ps
# Wait until both show "healthy"
```

### 2. Setup Hub (2 minutes)

```bash
cd hub
npm install
npx prisma migrate dev
npx prisma generate
npm run dev
```

Keep this terminal open. You should see:
```
▲ Next.js running on http://localhost:8080
```

### 3. Setup Worker (1 minute)

Open a **new terminal**:

```bash
cd /home/devyanshu/Desktop/email-validator/worker
go build -o validator-worker .
./validator-worker
```

You should see:
```
🚀 Starting Email Validator Worker...
✅ Connected to Redis
✅ Connected to PostgreSQL
✅ Started 50 workers
📬 Listening for emails in queue: email_queue
```

### 4. Test It! (30 seconds)

Open a **third terminal**:

```bash
# Create a job
curl -X POST http://localhost:8080/api/verify \
  -H "Content-Type: application/json" \
  -d '["test@example.com", "invalid@fake-domain-xyz.com"]'

# Copy the jobId from the response, then:
curl http://localhost:8080/api/job/YOUR_JOB_ID_HERE
```

Watch the worker terminal - you should see emails being processed!

## Troubleshooting

**Port 5432 in use?**
- Already handled - we use port 5433

**Can't connect to Redis?**
```bash
docker compose up -d redis
docker exec email-validator-redis redis-cli PING
```

**Can't connect to PostgreSQL?**
```bash
docker compose up -d postgres
# Wait 10 seconds for it to start
```

**Worker not processing?**
- Make sure Redis queue has items: `docker exec email-validator-redis redis-cli LLEN email_queue`
- Check worker is running: `ps aux | grep validator-worker`

## What's Next?

- Read the full [README.md](README.md) for detailed documentation
- Check API endpoints in the README
- Review safe limits and best practices

## Stopping Everything

```bash
# Stop worker: Ctrl+C in worker terminal
# Stop hub: Ctrl+C in hub terminal
# Stop infrastructure:
docker compose down
```
