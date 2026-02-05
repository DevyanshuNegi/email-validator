# Docker Compose Worker Setup

The Go worker can now be run either as a Docker container or standalone. This guide covers the Docker Compose approach.

## Quick Start

### Start Worker with Docker Compose

```bash
# Build and start the worker
docker compose up -d worker

# View logs
docker compose logs -f worker
```

### Stop Worker

```bash
docker compose stop worker
```

### Restart Worker

```bash
docker compose restart worker
```

## Configuration

The worker service in `docker-compose.yml` is configured with:

- **Auto-restart**: `restart: unless-stopped` (restarts on failure)
- **Health checks**: Waits for postgres, redis, and mailhog to be healthy
- **Environment variables**: All configurable via `.env` file or environment

## Environment Variables

Set these in your `.env` file (or pass via `docker compose`):

```bash
# Development mode (uses MailHog)
IS_DEV=true

# Database (auto-configured for Docker network)
DATABASE_URL=postgresql://postgres:postgres@postgres:5432/emailvalidator?sslmode=disable

# Redis (auto-configured for Docker network)
REDIS_ADDR=redis:6379
REDIS_PASSWORD=
REDIS_DB=0

# Worker hostname (for SMTP HELO)
WORKER_HOSTNAME=worker1.devyanshu.me

# SOCKS5 Proxy (optional, for production)
SOCKS5_PROXY=
PROXY_USER=
PROXY_PASS=
```

**Note**: In Docker Compose, the service names (`postgres`, `redis`) are used instead of `localhost` because they're on the same Docker network.

## Development vs Production

### Development Mode (Default)

```bash
# .env file
IS_DEV=true
```

- Uses MailHog for SMTP testing
- No proxy required
- Relaxed validation

### Production Mode

```bash
# .env file
IS_DEV=false
SOCKS5_PROXY=123.45.67.89:1080
PROXY_USER=your_username
PROXY_PASS=your_password
WORKER_HOSTNAME=worker1.devyanshu.me
```

- Uses real SMTP servers
- Requires SOCKS5 proxy
- Strict rate limiting (2 emails/sec global)

## Common Commands

```bash
# Build worker image (after code changes)
docker compose build worker

# Start worker
docker compose up -d worker

# Stop worker
docker compose stop worker

# View logs (follow mode)
docker compose logs -f worker

# View last 50 lines
docker compose logs --tail 50 worker

# Restart worker
docker compose restart worker

# Remove worker container
docker compose rm -f worker

# Rebuild and restart
docker compose build worker && docker compose up -d worker
```

## Troubleshooting

### Worker won't start

```bash
# Check if dependencies are healthy
docker compose ps

# Check worker logs
docker compose logs worker

# Verify Redis is accessible
docker compose exec worker sh -c "nc -z redis 6379 && echo 'Redis OK'"

# Verify PostgreSQL is accessible
docker compose exec worker sh -c "nc -z postgres 5432 && echo 'Postgres OK'"
```

### Worker exits immediately

```bash
# Check logs for errors
docker compose logs worker

# Common issues:
# - DATABASE_URL incorrect
# - REDIS_ADDR incorrect
# - Missing WORKER_HOSTNAME in production mode
```

### Rebuild after code changes

```bash
# Rebuild the image
docker compose build worker

# Restart the container
docker compose up -d worker
```

## Running Multiple Workers

To scale horizontally, you can run multiple worker instances:

```bash
# Start 3 worker instances
docker compose up -d --scale worker=3

# View logs from all instances
docker compose logs -f worker
```

**Note**: All workers share the same Redis queue, so they'll automatically distribute work.

## Standalone vs Docker

### Use Docker Compose when:
- ✅ Production deployment
- ✅ Need auto-restart on failure
- ✅ Want consistent environment
- ✅ Scaling multiple workers
- ✅ CI/CD pipelines

### Use Standalone when:
- ✅ Local development
- ✅ Quick debugging
- ✅ Direct terminal output
- ✅ Fast rebuild cycles

## Integration with Full Stack

To run everything together:

```bash
# Start all services (postgres, redis, mailhog, worker)
docker compose up -d

# Start Hub separately (Next.js)
cd hub
npm run dev
```

The worker will automatically connect to the other services via Docker's internal network.
