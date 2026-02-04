-- ====================================
-- GOVERSHOP - ADMIN SECURITY & TOTP
-- ====================================
-- Migration for admin manual topup security feature

-- 1. ADMIN SECURITY TABLE
-- Stores TOTP secret for admin (single admin setup)
CREATE TABLE IF NOT EXISTS admin_security (
    id SERIAL PRIMARY KEY,
    
    -- TOTP configuration
    totp_secret TEXT,                           -- Base32 encoded secret
    totp_enabled BOOLEAN DEFAULT false,
    
    -- For future multi-admin support
    admin_identifier VARCHAR(100) DEFAULT 'primary' UNIQUE,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- Insert default row for primary admin (safe insert)
INSERT INTO admin_security (admin_identifier, totp_enabled) 
VALUES ('primary', false)
ON CONFLICT (admin_identifier) DO NOTHING;

-- 2. ADMIN AUDIT LOGS TABLE
-- Tracks all sensitive admin actions
CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id SERIAL PRIMARY KEY,
    
    -- Action details
    action VARCHAR(50) NOT NULL,                -- 'manual_topup', 'totp_setup', 'totp_enable'
    order_id UUID,                              -- Reference to order (for manual_topup)
    
    -- Request details
    details JSONB,                              -- Additional context (SKU, customer_no, etc)
    ip_address VARCHAR(45),                     -- Client IP
    
    -- Result
    success BOOLEAN DEFAULT false,
    error_message TEXT,
    
    -- Timestamps
    created_at TIMESTAMP DEFAULT NOW()
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_action ON admin_audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_created ON admin_audit_logs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_admin_audit_logs_order ON admin_audit_logs(order_id);

-- Trigger for updated_at on admin_security
DROP TRIGGER IF EXISTS update_admin_security_updated_at ON admin_security;
CREATE TRIGGER update_admin_security_updated_at
    BEFORE UPDATE ON admin_security
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
