# IdeaBrowser Scraper

A comprehensive Go tool for scraping and archiving daily business ideas from IdeaBrowser.com, with SQLite storage and automated scheduling support.

## Prerequisites

- Go 1.19 or higher
- SQLite3 (for database storage)
- jq (for JSON parsing in scripts)
- An IdeaBrowser account (free registration at https://www.ideabrowser.com)

## Installation

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/zaakirio/ideabrowser-scraper
cd ideabrowser-scraper
```

2. Install dependencies:
```bash
go mod download
```

3. Copy and configure environment file:
```bash
cp .env.example .env
# Edit .env with your IdeaBrowser credentials
```

4. Build the scraper:
```bash
go build -o ideabrowser-scraper scraper.go
```

## Configuration

Edit `.env` and add your IdeaBrowser credentials:
```env
# Supabase Configuration (these are public and same for all users)
SUPABASE_ANON_KEY=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJpc3MiOiJzdXBhYmFzZSIsInJlZiI6ImNocWZ1bmF3Y2luaWVwYXF0ZGJkIiwicm9sZSI6ImFub24iLCJpYXQiOjE3NDQ4MDUyMDQsImV4cCI6MjA2MDM4MTIwNH0.3pIWFQSldqsWNl4biMKiKuLT3jUAzXxSaVh5SqAybc8
SUPABASE_PROJECT_URL=https://chqfunawciniepaqtdbd.supabase.co

# Your IdeaBrowser account credentials
IDEABROWSER_EMAIL=your_email@example.com
IDEABROWSER_PASSWORD=your_password
```

## Usage

### Manual Scraping

Run once to scrape today's idea:
```bash
./ideabrowser-scraper
```

With options:
```bash
# Verbose output
./ideabrowser-scraper -verbose

# Save to specific directory
./ideabrowser-scraper -output ./data/json

# Save HTML for debugging
./ideabrowser-scraper -save-html -output ./debug
```

### Database Storage

Import scraped JSON to SQLite:
```bash
# Import all JSON files
./scripts/ingest.sh

# Import specific file
./scripts/ingest.sh data/json/idea_*.json
```

Query the database:
```bash
./scripts/query.sh
```

## VPS Deployment & Automation

### Directory Structure

```
/opt/ideabrowser-scraper/
├── ideabrowser-scraper    # Compiled binary
├── .env                    # Credentials (chmod 600)
├── scripts/
│   ├── daily-scrape.sh    # Cron wrapper script
│   ├── ingest.sh          # JSON to SQLite importer
│   ├── query.sh           # Database query tool
│   └── schema.sql         # Database schema
└── data/
    ├── ideas.db           # SQLite database
    ├── json/              # JSON files archive
    └── logs/              # Execution logs
```

### Setup on VPS

1. **Deploy to VPS:**
```bash
# Clone or copy to VPS
cd /opt
git clone https://github.com/rubinkazan/ideabrowser-scraper
cd ideabrowser-scraper

# Build on server or upload pre-built binary
go build -o ideabrowser-scraper scraper.go

# Set up environment
cp .env.example .env
nano .env  # Add your credentials

# Secure the credentials
chmod 600 .env

# Make scripts executable
chmod +x scripts/*.sh

# Initialize database
sqlite3 data/ideas.db < scripts/schema.sql
```

2. **Configure Cron Job:**
```bash
# Edit crontab
crontab -e

# Add daily scrape at 2 AM
0 2 * * * /opt/ideabrowser-scraper/scripts/daily-scrape.sh
```

3. **Verify Setup:**
```bash
# Test run
/opt/ideabrowser-scraper/scripts/daily-scrape.sh

# Check logs
tail -f /opt/ideabrowser-scraper/data/logs/scraper-*.log

# Query database
/opt/ideabrowser-scraper/scripts/query.sh
```

## Database Schema

The SQLite database stores ideas with the following structure:

```sql
CREATE TABLE ideas (
    id INTEGER PRIMARY KEY,
    slug TEXT UNIQUE,
    scrape_date DATE,
    title TEXT,
    description TEXT,
    tags TEXT,
    value_equation_score INTEGER,
    acp_audience_score INTEGER,
    acp_community_score INTEGER,
    acp_product_score INTEGER,
    market_position TEXT,
    data JSON,  -- Full JSON data
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

### Query Examples

```sql
-- Find high-scoring ideas
SELECT slug, title, value_equation_score 
FROM ideas 
WHERE value_equation_score >= 8
ORDER BY scrape_date DESC;

-- Search by keyword
SELECT * FROM ideas 
WHERE title LIKE '%AI%' 
   OR description LIKE '%AI%';

-- Extract specific JSON fields
SELECT slug, 
       json_extract(data, '$.framework_fit.market_matrix.position') as position
FROM ideas;
```

## Scripts Reference

### `daily-scrape.sh`
Main automation script for cron jobs:
- Runs the scraper
- Imports JSON to SQLite
- Manages logs
- Handles errors

### `ingest.sh`
Imports JSON files to SQLite:
- Parses JSON and extracts key fields
- Handles duplicates (updates existing)
- Shows import statistics

### `query.sh`
Interactive database query tool:
- Browse all ideas
- Search by keyword
- Export to CSV
- Run custom SQL queries

## Monitoring

Check system health:
```bash
# View recent logs
tail -n 100 /opt/ideabrowser-scraper/data/logs/scraper-*.log

# Check database size
du -h /opt/ideabrowser-scraper/data/ideas.db

# Count total ideas
sqlite3 /opt/ideabrowser-scraper/data/ideas.db "SELECT COUNT(*) FROM ideas;"

# View recent scrapes
sqlite3 /opt/ideabrowser-scraper/data/ideas.db \
  "SELECT slug, scrape_date FROM ideas ORDER BY created_at DESC LIMIT 5;"
```

## Troubleshooting

### Authentication Errors
1. Verify credentials in `.env` file
2. Check if account is active on IdeaBrowser.com
3. Refresh tokens are saved in `refresh_token.txt`

### Cron Not Running
1. Check cron service: `systemctl status cron`
2. Verify script permissions: `ls -la scripts/`
3. Check cron logs: `grep CRON /var/log/syslog`

### Database Issues
1. Verify SQLite installed: `sqlite3 --version`
2. Check database integrity: `sqlite3 data/ideas.db "PRAGMA integrity_check;"`
3. Ensure jq is installed for JSON parsing: `jq --version`

## Disclaimer

This tool is for educational purposes. Please respect IdeaBrowser's terms of service and use responsibly. The tool only accesses data that you have legitimate access to with your account.
