CREATE TABLE IF NOT EXISTS brand_settings (
    brand_name VARCHAR(255) PRIMARY KEY,
    slug VARCHAR(255),
    custom_image_url TEXT,
    is_best_seller BOOLEAN DEFAULT FALSE,
    status VARCHAR(50) DEFAULT 'active', -- 'active', 'coming_soon', 'maintenance'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for faster lookup
CREATE INDEX idx_brand_settings_status ON brand_settings(status);
CREATE INDEX idx_brand_settings_best_seller ON brand_settings(is_best_seller);
