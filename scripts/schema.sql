-- IdeaBrowser SQLite Database Schema
-- Stores daily business ideas with full JSON data and searchable fields

CREATE TABLE IF NOT EXISTS ideas (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT UNIQUE NOT NULL,
    scrape_date DATE NOT NULL,
    title TEXT,
    description TEXT,
    tags TEXT, -- Comma-separated tags
    
    -- Framework scores for quick queries
    value_equation_score INTEGER,
    acp_audience_score INTEGER,
    acp_community_score INTEGER,
    acp_product_score INTEGER,
    market_position TEXT,
    
    -- Full JSON data
    data JSON NOT NULL,
    
    -- Metadata
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for common queries
CREATE INDEX IF NOT EXISTS idx_slug ON ideas(slug);
CREATE INDEX IF NOT EXISTS idx_date ON ideas(scrape_date);
CREATE INDEX IF NOT EXISTS idx_value_score ON ideas(value_equation_score);
CREATE INDEX IF NOT EXISTS idx_acp_scores ON ideas(acp_audience_score, acp_community_score, acp_product_score);

-- Trigger to update the updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_ideas_timestamp 
AFTER UPDATE ON ideas
BEGIN
    UPDATE ideas SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;