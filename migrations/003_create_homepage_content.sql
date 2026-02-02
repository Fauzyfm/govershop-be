-- Create homepage_content table for managing carousel, brand images, and popups
CREATE TABLE IF NOT EXISTS homepage_content (
    id SERIAL PRIMARY KEY,
    content_type VARCHAR(50) NOT NULL, -- 'carousel', 'brand_image', 'popup'
    brand_name VARCHAR(100), -- For brand_image type (e.g., 'MOBILE LEGENDS')
    image_url TEXT NOT NULL,
    title VARCHAR(255),
    description TEXT,
    link_url TEXT,
    sort_order INT DEFAULT 0,
    is_active BOOLEAN DEFAULT true,
    start_date TIMESTAMP,
    end_date TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Index for faster queries
CREATE INDEX idx_homepage_content_type ON homepage_content(content_type);
CREATE INDEX idx_homepage_content_brand ON homepage_content(brand_name);
CREATE INDEX idx_homepage_content_active ON homepage_content(is_active);
