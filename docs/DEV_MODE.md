# Development Mode Guide

This guide shows how to run the worker in development mode with **hot reload** - code changes instantly reflect in the Docker container.

## Quick Start

### Option 1: Using the Dev Script (Easiest)

```bash
./scripts/dev-worker.sh
```

This script:
- ✅ Checks and starts infrastructure (postgres, redis, mailhog)
- ✅ Starts worker with hot reload enabled
- ✅ Mounts source code as volume
- ✅ Auto-rebuilds on file changes

### Option 2: Manual Docker Compose

```bash
# Start infrastructure
docker compose up -d postgres redis mailhog

# Start worker with dev override
docker compose -f docker-compose.yml -f docker-compose.dev.yml up worker
```

### Option 3: Standalone with Air (No Docker)

If you have Go and `air` installed locally:

```bash
cd worker

# Install air (one-time)
go install github.com/cosmtrek/air@latest

# Run with hot reload
air
```

## How It Works

### Hot Reload with Air

The dev setup uses [Air](https://github.com/cosmtrek/air) - a live reload utility for Go:

1. **Watches** `.go` files for changes
2. **Rebuilds** automatically when files change
3. **Restarts** the worker process
4. **Shows** build errors in terminal

### Volume Mounts

In dev mode, the worker source code is mounted as a volume:
- ✅ Changes to `.go` files instantly visible in container
- ✅ No need to rebuild Docker image
- ✅ Fast iteration cycle

### Configuration

- **`.air.toml`**: Air configuration (what to watch, how to build)
- **`Dockerfile.dev`**: Development Dockerfile with Air pre-installed
- **`docker-compose.dev.yml`**: Dev override with volume mounts

## File Structure

```
worker/
├── Dockerfile          # Production build
├── Dockerfile.dev      # Development with Air
├── .air.toml          # Air hot reload config
├── main.go
├── smtp.go
└── ratelimiter.go
```

## Development Workflow

1. **Start dev worker:**
   ```bash
   ./scripts/dev-worker.sh
   ```

2. **Edit code** in `worker/*.go` files

3. **Watch auto-reload:**
   ```
   worker-1  | 2026/02/05 18:30:15 Running...
   worker-1  | main.go has changed
   worker-1  | Building...
   worker-1  | Running...
   worker-1  | 🚀 Starting Email Validator Worker...
   ```

4. **Test changes** - worker restarts automatically!

## Troubleshooting

### Air not detecting changes

```bash
# Check if volume is mounted correctly
docker compose -f docker-compose.yml -f docker-compose.dev.yml exec worker ls -la /app

# Verify file permissions
docker compose -f docker-compose.yml -f docker-compose.dev.yml exec worker ls -la /app/*.go
```

### Build errors not showing

Check Air logs:
```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml logs worker
```

### Container exits immediately

Check if Air is installed:
```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml exec worker which air
```

If missing, rebuild:
```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml build worker
```

### Slow rebuilds

Air rebuilds on every save. For faster iteration:
- Use standalone `air` (no Docker overhead)
- Or reduce `delay` in `.air.toml`

## Customizing Air

Edit `worker/.air.toml` to customize:

```toml
[build]
  delay = 1000        # Wait 1s after change before rebuilding
  exclude_dir = [...] # Directories to ignore
  include_ext = [...]  # File extensions to watch
```

## Switching Between Modes

### Dev Mode (Hot Reload)
```bash
docker compose -f docker-compose.yml -f docker-compose.dev.yml up worker
```

### Production Mode (No Hot Reload)
```bash
docker compose up -d worker
```

### Standalone (Local)
```bash
cd worker
./validator-worker
```

## Tips

1. **Keep terminal open** - Watch Air rebuild logs
2. **Use `.air.toml`** - Customize what Air watches
3. **Check logs** - `docker compose logs -f worker`
4. **Test quickly** - Changes reflect in ~1-2 seconds

## Performance

- **First build**: ~10-15 seconds (normal Go compile)
- **Incremental builds**: ~1-2 seconds (only changed files)
- **Hot reload delay**: 1 second (configurable)

## Next Steps

- Read [DOCKER_WORKER.md](DOCKER_WORKER.md) for production setup
- Check [QUICKSTART.md](QUICKSTART.md) for full stack setup
- Review [README.md](../README.md) for architecture details
