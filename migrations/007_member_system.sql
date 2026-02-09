-- ====================================
-- MEMBER SYSTEM MIGRATION
-- ====================================
-- Adds users and deposits tables for member/reseller system

-- ====================================
-- 1. USERS TABLE
-- ====================================
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(100) UNIQUE NOT NULL,
    password VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE DEFAULT NULL,
    full_name VARCHAR(255) NOT NULL,
    role VARCHAR(50) DEFAULT 'member',      -- member, admin
    balance NUMERIC(15, 2) DEFAULT 0,
    status VARCHAR(50) DEFAULT 'active',    -- active, suspended
    whatsapp VARCHAR(20) DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_role ON users(role);
CREATE INDEX idx_users_status ON users(status);

-- ====================================
-- 2. DEPOSITS TABLE (Balance History)
-- ====================================
CREATE TABLE IF NOT EXISTS deposits (
    id SERIAL PRIMARY KEY,
    user_id INT REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(15, 2) NOT NULL,
    type VARCHAR(50) NOT NULL,              -- credit (topup), debit (purchase), refund
    description TEXT,
    reference_id VARCHAR(100) DEFAULT NULL, -- order_id, payment_id
    status VARCHAR(50) DEFAULT 'success',   -- success, pending, failed
    created_by VARCHAR(100) DEFAULT 'system',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_deposits_user_id ON deposits(user_id);
CREATE INDEX idx_deposits_type ON deposits(type);
CREATE INDEX idx_deposits_created_at ON deposits(created_at DESC);

-- ====================================
-- 3. ADD MEMBER COLUMNS TO PRODUCTS
-- ====================================
ALTER TABLE products ADD COLUMN IF NOT EXISTS member_markup_percent DECIMAL(5,2) DEFAULT NULL;

-- ====================================
-- 4. ADD MEMBER COLUMNS TO ORDERS
-- ====================================
ALTER TABLE orders ADD COLUMN IF NOT EXISTS member_id INT REFERENCES users(id);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS member_price DECIMAL(15,2);

-- Index for member orders
CREATE INDEX idx_orders_member_id ON orders(member_id);

-- ====================================
-- 5. TRIGGER FOR USERS updated_at
-- ====================================
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
