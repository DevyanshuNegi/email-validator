# Project Structure

This document describes the organized file structure of the Email Validator project.

## Directory Structure

```
email-validator/
├── hub/                    # Next.js API Hub
│   ├── src/
│   │   ├── app/           # Next.js App Router
│   │   │   └── api/       # API routes
│   │   └── lib/           # Utilities (Redis, Prisma)
│   └── prisma/            # Database schema and migrations
│
├── worker/                 # Go Email Validator Worker
│   ├── main.go            # Main entry point
│   ├── smtp.go            # SMTP validation logic
│   ├── ratelimiter.go     # Rate limiting
│   └── *.md               # Worker documentation
│
├── scripts/               # Executable scripts
│   ├── test-suite.sh      # Main test suite
│   └── test-enterprise-features.sh
│
├── tests/                 # Test files and data
│   ├── data/              # Test data (CSV files)
│   │   ├── test-emails.csv
│   │   └── test-emails-small.csv
│   ├── results/           # Test results (generated)
│   │   ├── test-results-*.csv
│   │   ├── test-results-*.json
│   │   └── test-summary-*.txt
│   ├── test-performance.py
│   └── requirements-test.txt
│
├── logs/                  # Application logs
│   └── test-errors-*.log
│
├── docs/                  # Documentation
│   ├── STRUCTURE.md       # This file
│   ├── QUICKSTART.md
│   ├── test-findings.md
│   └── project-summary-*.md
│
├── docker-compose.yml     # Docker services
└── README.md             # Main project README
```

## Directory Purposes

### `/hub`
Next.js application serving the API and frontend.
- API routes: `/api/verify`, `/api/job/[id]`
- Database: Prisma ORM with PostgreSQL
- Queue: Redis connection for job queueing

### `/worker`
Go worker that processes email validation jobs.
- Main logic: SMTP validation, rate limiting, proxy support
- Concurrency: 50 goroutines processing Redis queue
- Safety: Rate limiting, SOCKS5 proxy, proper SMTP identity

### `/scripts`
Executable shell scripts for testing and automation.
- `test-suite.sh`: Comprehensive test suite
- `test-enterprise-features.sh`: Enterprise feature tests

### `/tests`
Test files, test data, and test results.
- `data/`: Input test data (CSV files with email lists)
- `results/`: Generated test results (CSV, JSON, summaries)
- `test-performance.py`: Python performance testing script

### `/logs`
Application and test logs.
- Test error logs with timestamps
- Worker logs (if redirected)
- API logs (if redirected)

### `/docs`
Project documentation.
- Architecture documentation
- Quick start guides
- Test findings and summaries
- Project summaries

## File Naming Conventions

### Test Results
- Format: `test-results-YYYYMMDD_HHMMSS.{csv,json}`
- Summaries: `test-summary-YYYYMMDD_HHMMSS.txt`

### Logs
- Format: `test-errors-YYYYMMDD_HHMMSS.log`
- Application logs: `app-YYYYMMDD.log` (if implemented)

### Scripts
- All scripts are executable (`.sh` extension)
- Use descriptive names: `test-*-features.sh`

## Git Ignore

The following are ignored:
- `logs/*.log` and `logs/*.txt` (except `.gitkeep`)
- `tests/results/*` (except `.gitkeep`)
- `node_modules/`
- `.next/`
- `generated/`

## Adding New Files

### New Test Script
Place in `/scripts` and make executable:
```bash
chmod +x scripts/my-test.sh
```

### New Test Data
Place in `/tests/data/`:
```bash
cp my-emails.csv tests/data/
```

### New Documentation
Place in `/docs/`:
```bash
cp my-doc.md docs/
```

### New Logs
Logs are automatically placed in `/logs/` by scripts, or redirect manually:
```bash
./my-script.sh 2>&1 | tee logs/my-script-$(date +%Y%m%d_%H%M%S).log
```

## Maintenance

### Cleaning Old Logs
```bash
# Remove logs older than 30 days
find logs/ -name "*.log" -mtime +30 -delete
```

### Cleaning Old Test Results
```bash
# Remove test results older than 7 days
find tests/results/ -name "test-*" -mtime +7 -delete
```

### Archiving
```bash
# Archive old logs
tar -czf logs-archive-$(date +%Y%m%d).tar.gz logs/*.log
rm logs/*.log
```
