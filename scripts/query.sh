#!/bin/bash

# Query Helper Script for IdeaBrowser SQLite Database
# Provides easy access to common queries

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
DB_PATH="${DB_PATH:-$PROJECT_DIR/data/ideas.db}"

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

# Check if database exists
if [ ! -f "$DB_PATH" ]; then
    echo "Database not found at $DB_PATH"
    echo "Run the scraper first to create the database."
    exit 1
fi

show_menu() {
    echo -e "${BLUE}IdeaBrowser Database Query Tool${NC}"
    echo "================================"
    echo "1. Show all ideas (summary)"
    echo "2. Show recent ideas (last 7 days)"
    echo "3. Show top rated ideas (by value equation score)"
    echo "4. Search by keyword"
    echo "5. Show idea details by slug"
    echo "6. Export all to CSV"
    echo "7. Show database statistics"
    echo "8. Custom SQL query"
    echo "0. Exit"
    echo
}

while true; do
    show_menu
    read -p "Select option: " choice
    
    case $choice in
        1)
            echo -e "\n${GREEN}All Ideas:${NC}"
            sqlite3 "$DB_PATH" -column -header <<EOF
SELECT slug, scrape_date, title, value_equation_score 
FROM ideas 
ORDER BY scrape_date DESC;
EOF
            ;;
            
        2)
            echo -e "\n${GREEN}Recent Ideas (Last 7 Days):${NC}"
            sqlite3 "$DB_PATH" -column -header <<EOF
SELECT slug, scrape_date, title, value_equation_score 
FROM ideas 
WHERE scrape_date >= date('now', '-7 days')
ORDER BY scrape_date DESC;
EOF
            ;;
            
        3)
            echo -e "\n${GREEN}Top Rated Ideas:${NC}"
            sqlite3 "$DB_PATH" -column -header <<EOF
SELECT slug, title, value_equation_score,
       acp_audience_score, acp_community_score, acp_product_score
FROM ideas 
WHERE value_equation_score > 0
ORDER BY value_equation_score DESC 
LIMIT 10;
EOF
            ;;
            
        4)
            read -p "Enter search keyword: " keyword
            echo -e "\n${GREEN}Search Results for '$keyword':${NC}"
            sqlite3 "$DB_PATH" -column -header <<EOF
SELECT slug, scrape_date, title 
FROM ideas 
WHERE title LIKE '%$keyword%' 
   OR description LIKE '%$keyword%'
   OR tags LIKE '%$keyword%'
ORDER BY scrape_date DESC;
EOF
            ;;
            
        5)
            read -p "Enter idea slug: " slug
            echo -e "\n${GREEN}Details for $slug:${NC}"
            sqlite3 "$DB_PATH" <<EOF
SELECT json_pretty(data) 
FROM ideas 
WHERE slug = '$slug';
EOF
            ;;
            
        6)
            output_file="$PROJECT_DIR/data/ideas_export_$(date +%Y%m%d).csv"
            echo -e "\n${GREEN}Exporting to CSV: $output_file${NC}"
            sqlite3 "$DB_PATH" -csv -header <<EOF > "$output_file"
SELECT slug, scrape_date, title, description, tags,
       value_equation_score, acp_audience_score, 
       acp_community_score, acp_product_score, market_position
FROM ideas 
ORDER BY scrape_date DESC;
EOF
            echo "Export complete!"
            ;;
            
        7)
            echo -e "\n${GREEN}Database Statistics:${NC}"
            sqlite3 "$DB_PATH" <<EOF
SELECT 'Total Ideas' as metric, COUNT(*) as value FROM ideas
UNION ALL
SELECT 'Date Range', MIN(scrape_date) || ' to ' || MAX(scrape_date) FROM ideas
UNION ALL
SELECT 'Avg Value Score', ROUND(AVG(value_equation_score), 2) FROM ideas WHERE value_equation_score > 0
UNION ALL
SELECT 'Database Size', ROUND(page_count * page_size / 1024.0 / 1024.0, 2) || ' MB' FROM pragma_page_count(), pragma_page_size();
EOF
            ;;
            
        8)
            echo "Enter SQL query (or 'cancel' to go back):"
            read -p "> " query
            if [ "$query" != "cancel" ]; then
                sqlite3 "$DB_PATH" -column -header "$query"
            fi
            ;;
            
        0)
            echo "Goodbye!"
            exit 0
            ;;
            
        *)
            echo "Invalid option"
            ;;
    esac
    
    echo
    read -p "Press Enter to continue..."
    clear
done