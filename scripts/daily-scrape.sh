#!/bin/bash

# Daily Scrape Script for IdeaBrowser
# This script is designed to be run by cron daily

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
SCRAPER_BIN="$PROJECT_DIR/ideabrowser-scraper"
JSON_DIR="$PROJECT_DIR/data/json"
LOG_DIR="$PROJECT_DIR/data/logs"
LOG_FILE="$LOG_DIR/scraper-$(date +%Y-%m).log"

# Create necessary directories
mkdir -p "$JSON_DIR"
mkdir -p "$LOG_DIR"

# Function to log with timestamp
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# Start scraping
log "========================================="
log "Starting daily IdeaBrowser scrape"
log "========================================="

# Change to project directory
cd "$PROJECT_DIR" || exit 1

# Check if scraper binary exists
if [ ! -f "$SCRAPER_BIN" ]; then
    log "ERROR: Scraper binary not found at $SCRAPER_BIN"
    log "Building scraper..."
    go build -o "$SCRAPER_BIN" scraper.go
    if [ $? -ne 0 ]; then
        log "ERROR: Failed to build scraper"
        exit 1
    fi
fi

# Run the scraper
log "Running scraper..."
"$SCRAPER_BIN" -output "$JSON_DIR" -verbose 2>&1 | tee -a "$LOG_FILE"
SCRAPER_EXIT_CODE=${PIPESTATUS[0]}

if [ $SCRAPER_EXIT_CODE -eq 0 ]; then
    log "Scraper completed successfully"
    
    # Find the JSON file created today
    TODAY_JSON=$(find "$JSON_DIR" -name "idea_*_$(date +%Y-%m-%d).json" -type f -mtime -1 | head -1)
    
    if [ -n "$TODAY_JSON" ]; then
        log "Found today's JSON: $(basename "$TODAY_JSON")"
        
        # Import to SQLite
        log "Importing to SQLite database..."
        "$SCRIPT_DIR/ingest.sh" "$TODAY_JSON" 2>&1 | tee -a "$LOG_FILE"
        
        if [ ${PIPESTATUS[0]} -eq 0 ]; then
            log "Import completed successfully"
        else
            log "ERROR: Import to SQLite failed"
            exit 1
        fi
    else
        log "WARNING: No JSON file found for today"
    fi
else
    log "ERROR: Scraper failed with exit code $SCRAPER_EXIT_CODE"
    exit 1
fi

# Rotate logs if they get too large (>10MB)
if [ -f "$LOG_FILE" ]; then
    LOG_SIZE=$(stat -f%z "$LOG_FILE" 2>/dev/null || stat -c%s "$LOG_FILE" 2>/dev/null)
    if [ "$LOG_SIZE" -gt 10485760 ]; then
        log "Rotating log file (size: $LOG_SIZE bytes)"
        mv "$LOG_FILE" "$LOG_FILE.$(date +%Y%m%d%H%M%S)"
        touch "$LOG_FILE"
    fi
fi

log "Daily scrape completed"
log "========================================="

# Optional: Send notification on error
# if [ $SCRAPER_EXIT_CODE -ne 0 ]; then
#     echo "IdeaBrowser scraper failed. Check logs at $LOG_FILE" | mail -s "Scraper Error" admin@example.com
# fi

exit 0