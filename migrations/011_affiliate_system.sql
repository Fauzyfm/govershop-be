-- Migration: Affiliate Partnership System
-- Adds affiliate tracking columns to orders table and affiliate_balance to users table

-- Add affiliate columns to orders table (if not exist)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='orders' AND column_name='affiliate_id') THEN
        ALTER TABLE orders ADD COLUMN affiliate_id INTEGER REFERENCES affiliate_partners(id);
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='orders' AND column_name='affiliate_channel') THEN
        ALTER TABLE orders ADD COLUMN affiliate_channel VARCHAR(10); -- 'link' or 'code'
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='orders' AND column_name='affiliate_discount') THEN
        ALTER TABLE orders ADD COLUMN affiliate_discount DECIMAL(15,2) DEFAULT 0;
    END IF;
END $$;

-- Add affiliate_balance to users table (if not exist)
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='users' AND column_name='affiliate_balance') THEN
        ALTER TABLE users ADD COLUMN affiliate_balance DECIMAL(15,2) DEFAULT 0;
    END IF;
END $$;

-- Create index for faster affiliate lookups on orders
CREATE INDEX IF NOT EXISTS idx_orders_affiliate_id ON orders(affiliate_id);
CREATE INDEX IF NOT EXISTS idx_affiliate_usages_affiliate_id ON affiliate_usages(affiliate_id);
CREATE INDEX IF NOT EXISTS idx_affiliate_usages_customer_no ON affiliate_usages(customer_no);
CREATE INDEX IF NOT EXISTS idx_affiliate_usages_created_at ON affiliate_usages(created_at);
CREATE INDEX IF NOT EXISTS idx_affiliate_partners_code ON affiliate_partners(code);
CREATE INDEX IF NOT EXISTS idx_affiliate_partners_user_id ON affiliate_partners(user_id);
