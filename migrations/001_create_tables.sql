-- ====================================
-- GOVERSHOP DATABASE SCHEMA
-- ====================================
-- Run this migration to create all required tables

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ====================================
-- 1. PRODUCTS TABLE
-- ====================================
-- Cached products from Digiflazz with pricing
CREATE TABLE IF NOT EXISTS products (
    id SERIAL PRIMARY KEY,
    
    -- Product identification (from Digiflazz)
    buyer_sku_code VARCHAR(50) UNIQUE NOT NULL,
    product_name VARCHAR(255) NOT NULL,
    category VARCHAR(100),
    brand VARCHAR(100),
    type VARCHAR(100),
    seller_name VARCHAR(255),
    
    -- Pricing
    buy_price DECIMAL(15,2) NOT NULL,           -- Harga beli dari Digiflazz
    markup_percent DECIMAL(5,2) DEFAULT 3.00,   -- Persentase markup (bisa custom per produk)
    selling_price DECIMAL(15,2) NOT NULL,       -- Harga jual ke customer
    discount_price DECIMAL(15,2),               -- Harga promo (nullable)
    
    -- Availability
    is_available BOOLEAN DEFAULT true,
    buyer_product_status BOOLEAN DEFAULT true,  -- Status dari Digiflazz
    seller_product_status BOOLEAN DEFAULT true, -- Status seller dari Digiflazz
    unlimited_stock BOOLEAN DEFAULT false,
    stock INTEGER DEFAULT 0,
    
    -- Additional info
    description TEXT,
    start_cut_off VARCHAR(10),                  -- Jam mulai cut off (e.g., "23:45")
    end_cut_off VARCHAR(10),                    -- Jam selesai cut off (e.g., "00:15")
    is_multi BOOLEAN DEFAULT false,             -- Support multiple transactions
    
    -- Timestamps
    last_sync_at TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Indexes for fast queries
CREATE INDEX idx_products_category ON products(category);
CREATE INDEX idx_products_brand ON products(brand);
CREATE INDEX idx_products_available ON products(is_available);
CREATE INDEX idx_products_sku ON products(buyer_sku_code);

-- ====================================
-- 2. ORDERS TABLE
-- ====================================
-- Customer orders
CREATE TABLE IF NOT EXISTS orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Reference IDs
    ref_id VARCHAR(100) UNIQUE NOT NULL,        -- Unique ID untuk Digiflazz (format: GVS-{timestamp}-{random})
    
    -- Product info (snapshot at order time)
    buyer_sku_code VARCHAR(50) NOT NULL,
    product_name VARCHAR(255),
    customer_no VARCHAR(100) NOT NULL,          -- Nomor HP / ID tujuan topup
    
    -- Pricing (snapshot at order time)
    buy_price DECIMAL(15,2),                    -- Harga beli saat order
    selling_price DECIMAL(15,2),                -- Harga jual saat order
    
    -- Status tracking
    status VARCHAR(50) DEFAULT 'pending',       -- pending, waiting_payment, paid, processing, success, failed, cancelled, refunded
    
    -- Digiflazz response
    digiflazz_status VARCHAR(50),               -- Pending, Sukses, Gagal
    digiflazz_rc VARCHAR(10),                   -- Response code dari Digiflazz
    serial_number VARCHAR(255),                 -- SN dari Digiflazz (bukti topup)
    digiflazz_message TEXT,                     -- Message dari Digiflazz
    
    -- Customer info (optional, for notifications)
    customer_email VARCHAR(255),
    customer_phone VARCHAR(20),
    customer_name VARCHAR(255),
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP                      -- Waktu transaksi selesai
);

-- Indexes
CREATE INDEX idx_orders_ref_id ON orders(ref_id);
CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_customer_no ON orders(customer_no);
CREATE INDEX idx_orders_created_at ON orders(created_at DESC);

-- ====================================
-- 3. PAYMENTS TABLE
-- ====================================
-- Payment transactions via Pakasir
CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Relations
    order_id UUID REFERENCES orders(id) ON DELETE CASCADE,
    
    -- Payment details
    amount DECIMAL(15,2) NOT NULL,              -- Harga produk
    fee DECIMAL(15,2) DEFAULT 0,                -- Biaya payment gateway
    total_payment DECIMAL(15,2) NOT NULL,       -- Total yang harus dibayar
    
    -- Payment method
    payment_method VARCHAR(50) NOT NULL,        -- qris, bni_va, bri_va, etc
    payment_number TEXT,                        -- QR string atau nomor VA
    
    -- Status
    status VARCHAR(50) DEFAULT 'pending',       -- pending, completed, expired, cancelled
    
    -- Timestamps
    expired_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_payments_order_id ON payments(order_id);
CREATE INDEX idx_payments_status ON payments(status);

-- ====================================
-- 4. PROMOS TABLE
-- ====================================
-- Discount and promo events
CREATE TABLE IF NOT EXISTS promos (
    id SERIAL PRIMARY KEY,
    
    -- Promo info
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) UNIQUE,                    -- Kode promo (optional)
    description TEXT,
    
    -- Discount configuration
    discount_type VARCHAR(20) NOT NULL,         -- 'percent' atau 'fixed'
    discount_value DECIMAL(15,2) NOT NULL,      -- Nilai diskon (% atau nominal)
    
    -- Limits
    min_purchase DECIMAL(15,2) DEFAULT 0,       -- Minimum pembelian
    max_discount DECIMAL(15,2),                 -- Maksimum potongan (untuk percent)
    usage_limit INTEGER,                        -- Batas penggunaan total
    usage_count INTEGER DEFAULT 0,              -- Jumlah sudah digunakan
    
    -- Targeting (null = apply to all)
    category VARCHAR(100),                      -- Apply ke kategori tertentu
    brand VARCHAR(100),                         -- Apply ke brand tertentu
    buyer_sku_code VARCHAR(50),                 -- Apply ke produk tertentu
    
    -- Validity period
    start_date TIMESTAMP NOT NULL,
    end_date TIMESTAMP NOT NULL,
    
    -- Status
    is_active BOOLEAN DEFAULT true,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_promos_active ON promos(is_active, start_date, end_date);
CREATE INDEX idx_promos_code ON promos(code);

-- ====================================
-- 5. WEBHOOK LOGS TABLE
-- ====================================
-- Log semua webhook untuk debugging
CREATE TABLE IF NOT EXISTS webhook_logs (
    id SERIAL PRIMARY KEY,
    
    source VARCHAR(50) NOT NULL,                -- 'pakasir' atau 'digiflazz'
    payload JSONB NOT NULL,                     -- Raw JSON payload
    processed BOOLEAN DEFAULT false,
    error_message TEXT,
    
    created_at TIMESTAMP DEFAULT NOW()
);

-- Index
CREATE INDEX idx_webhook_logs_source ON webhook_logs(source, created_at DESC);

-- ====================================
-- 6. SYNC LOGS TABLE
-- ====================================
-- Log product sync history
CREATE TABLE IF NOT EXISTS sync_logs (
    id SERIAL PRIMARY KEY,
    
    sync_type VARCHAR(50) NOT NULL,             -- 'products', 'prepaid', 'pasca'
    total_products INTEGER DEFAULT 0,
    new_products INTEGER DEFAULT 0,
    updated_products INTEGER DEFAULT 0,
    failed_products INTEGER DEFAULT 0,
    
    status VARCHAR(20) DEFAULT 'running',       -- running, success, failed
    error_message TEXT,
    
    started_at TIMESTAMP DEFAULT NOW(),
    completed_at TIMESTAMP
);

-- ====================================
-- HELPER FUNCTIONS
-- ====================================

-- Function to auto-update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to tables with updated_at
CREATE TRIGGER update_products_updated_at
    BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_orders_updated_at
    BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_promos_updated_at
    BEFORE UPDATE ON promos
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
