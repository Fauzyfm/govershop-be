-- ====================================
-- GOVERSHOP - ORDER SOURCE COLUMN
-- ====================================
-- Adds order_source to distinguish between regular orders and admin topups

-- Add order_source column
ALTER TABLE orders 
ADD COLUMN IF NOT EXISTS order_source VARCHAR(20) DEFAULT 'website';

-- Add comment
COMMENT ON COLUMN orders.order_source IS 'Source: website (customer), admin_cash (cash payment), admin_gift (free gift)';

-- Add notes column for admin topups (optional reason/note)
ALTER TABLE orders
ADD COLUMN IF NOT EXISTS admin_notes TEXT;
