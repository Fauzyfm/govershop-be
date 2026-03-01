-- Display Categories: custom categories for frontend tabs
CREATE TABLE IF NOT EXISTS display_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE,
    sort_order INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add display_category and display_sort_order to brand_settings
ALTER TABLE brand_settings 
    ADD COLUMN IF NOT EXISTS display_category VARCHAR(100) DEFAULT NULL;

ALTER TABLE brand_settings 
    ADD COLUMN IF NOT EXISTS display_sort_order INT DEFAULT 0;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_brand_settings_display_cat ON brand_settings(display_category);
CREATE INDEX IF NOT EXISTS idx_display_categories_sort ON display_categories(sort_order);
