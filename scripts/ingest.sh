#!/bin/bash

# IdeaBrowser JSON to SQLite Ingestion Script
# Imports scraped JSON data into SQLite database

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DB_PATH="${DB_PATH:-$PROJECT_DIR/data/ideas.db}"
JSON_DIR="${JSON_DIR:-$PROJECT_DIR/data/json}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to log messages
log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" >&2
}

warning() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1"
}

# Check if sqlite3 is installed
if ! command -v sqlite3 &> /dev/null; then
    error "sqlite3 is not installed. Please install it first."
    exit 1
fi

# Check if jq is installed for JSON parsing
if ! command -v jq &> /dev/null; then
    error "jq is not installed. Please install it for JSON parsing."
    exit 1
fi

# Create data directory if it doesn't exist
mkdir -p "$(dirname "$DB_PATH")"
mkdir -p "$JSON_DIR"

# Initialize database if it doesn't exist
if [ ! -f "$DB_PATH" ]; then
    log "Creating new database at $DB_PATH"
    sqlite3 "$DB_PATH" < "$SCRIPT_DIR/schema.sql"
    if [ $? -eq 0 ]; then
        log "Database created successfully"
    else
        error "Failed to create database"
        exit 1
    fi
fi

# Function to import a single JSON file
import_json() {
    local json_file="$1"
    local filename=$(basename "$json_file")
    
    log "Processing $filename"
    
    # Check if file exists and is readable
    if [ ! -r "$json_file" ]; then
        error "Cannot read file: $json_file"
        return 1
    fi
    
    # Extract data from JSON using jq
    local slug=$(jq -r '.slug // empty' "$json_file")
    local title=$(jq -r '.title // empty' "$json_file")
    local description=$(jq -r '.description // empty' "$json_file")
    local date=$(jq -r '.date // empty' "$json_file")
    local tags=$(jq -r '.tags // [] | join(",")' "$json_file")
    
    # Extract framework scores
    local value_score=$(jq -r '.framework_fit.value_equation.score // 0' "$json_file")
    local audience_score=$(jq -r '.framework_fit.acp_framework.audience_score // 0' "$json_file")
    local community_score=$(jq -r '.framework_fit.acp_framework.community_score // 0' "$json_file")
    local product_score=$(jq -r '.framework_fit.acp_framework.product_score // 0' "$json_file")
    local market_position=$(jq -r '.framework_fit.market_matrix.position // empty' "$json_file")
    
    # Get the full JSON data
    local json_data=$(cat "$json_file")
    
    # Parse date to ISO format (handle "Jan 17, 2025" format)
    if [ -n "$date" ]; then
        scrape_date=$(date -d "$date" '+%Y-%m-%d' 2>/dev/null || echo "$date")
    else
        # Try to extract date from filename (e.g., idea_slug_2025-01-17.json)
        scrape_date=$(echo "$filename" | grep -oE '[0-9]{4}-[0-9]{2}-[0-9]{2}' | head -1)
    fi
    
    if [ -z "$slug" ]; then
        warning "No slug found in $filename, skipping"
        return 1
    fi
    
    # Check if idea already exists
    exists=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM ideas WHERE slug = '$slug';")
    
    if [ "$exists" -gt 0 ]; then
        warning "Idea with slug '$slug' already exists, updating..."
        
        # Update existing record
        sqlite3 "$DB_PATH" <<EOF
UPDATE ideas SET
    title = '$title',
    description = '$description',
    scrape_date = '$scrape_date',
    tags = '$tags',
    value_equation_score = $value_score,
    acp_audience_score = $audience_score,
    acp_community_score = $community_score,
    acp_product_score = $product_score,
    market_position = '$market_position',
    data = json('$json_data'),
    updated_at = CURRENT_TIMESTAMP
WHERE slug = '$slug';
EOF
        
        if [ $? -eq 0 ]; then
            log "Updated: $slug"
        else
            error "Failed to update $slug"
            return 1
        fi
    else
        # Insert new record
        sqlite3 "$DB_PATH" <<EOF
INSERT INTO ideas (
    slug, title, description, scrape_date, tags,
    value_equation_score, acp_audience_score, acp_community_score, 
    acp_product_score, market_position, data
) VALUES (
    '$slug', '$title', '$description', '$scrape_date', '$tags',
    $value_score, $audience_score, $community_score, 
    $product_score, '$market_position', json('$json_data')
);
EOF
        
        if [ $? -eq 0 ]; then
            log "Imported: $slug"
        else
            error "Failed to import $slug"
            return 1
        fi
    fi
    
    return 0
}

# Main execution
main() {
    log "Starting JSON to SQLite ingestion"
    log "Database: $DB_PATH"
    log "JSON Directory: $JSON_DIR"
    
    # If a specific JSON file is provided as argument
    if [ $# -gt 0 ]; then
        for json_file in "$@"; do
            import_json "$json_file"
        done
    else
        # Process all JSON files in the directory
        json_count=0
        success_count=0
        
        for json_file in "$JSON_DIR"/*.json; do
            if [ -f "$json_file" ]; then
                json_count=$((json_count + 1))
                if import_json "$json_file"; then
                    success_count=$((success_count + 1))
                fi
            fi
        done
        
        if [ $json_count -eq 0 ]; then
            warning "No JSON files found in $JSON_DIR"
        else
            log "Processed $success_count/$json_count files successfully"
        fi
    fi
    
    # Show database statistics
    total_ideas=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM ideas;")
    log "Total ideas in database: $total_ideas"
    
    # Show recent ideas
    log "Recent ideas:"
    sqlite3 "$DB_PATH" -column -header <<EOF
SELECT slug, scrape_date, value_equation_score 
FROM ideas 
ORDER BY scrape_date DESC 
LIMIT 5;
EOF
}

# Run main function
main "$@"