-- =====================================================
-- Migration: Set PostgreSQL Timezone to WIB (Asia/Jakarta)
-- dan Konversi semua data timestamp yang sudah ada
-- =====================================================
-- Jalankan di DBeaver, satu per satu:

-- ==========================================
-- STEP 1: Set timezone database (WAJIB)
-- ==========================================
ALTER DATABASE govershop SET timezone TO 'Asia/Jakarta';

-- Setelah menjalankan STEP 1, RECONNECT ke database di DBeaver,
-- lalu verifikasi dengan:
-- SHOW timezone;
-- Harusnya menampilkan: Asia/Jakarta

-- ==========================================
-- STEP 2: Konversi semua data yang sudah ada
-- dari UTC ke WIB (+7 jam)
-- ⚠️ HANYA JALANKAN SEKALI!
-- ==========================================

-- 2a. Tabel orders
UPDATE orders SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours',
    completed_at = completed_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2b. Tabel users
UPDATE users SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2c. Tabel deposits
UPDATE deposits SET 
    created_at = created_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2d. Tabel products
UPDATE products SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours',
    last_sync_at = last_sync_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2e. Tabel payments
UPDATE payments SET 
    created_at = created_at + INTERVAL '7 hours',
    expired_at = expired_at + INTERVAL '7 hours',
    completed_at = completed_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2f. Tabel webhook_logs
UPDATE webhook_logs SET 
    created_at = created_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2g. Tabel sync_logs
UPDATE sync_logs SET 
    started_at = started_at + INTERVAL '7 hours',
    completed_at = completed_at + INTERVAL '7 hours'
WHERE started_at IS NOT NULL;

-- 2h. Tabel homepage_content
UPDATE homepage_content SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2i. Tabel brand_settings
UPDATE brand_settings SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2j. Tabel admin_security
UPDATE admin_security SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2k. Tabel admin_audit_logs
UPDATE admin_audit_logs SET 
    created_at = created_at + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- 2l. Tabel promos (jika ada data)
UPDATE promos SET 
    created_at = created_at + INTERVAL '7 hours',
    updated_at = updated_at + INTERVAL '7 hours',
    start_date = start_date + INTERVAL '7 hours',
    end_date = end_date + INTERVAL '7 hours'
WHERE created_at IS NOT NULL;

-- ==========================================
-- STEP 3: Verifikasi (cek data setelahnya)
-- ==========================================
-- SELECT id, ref_id, created_at FROM orders ORDER BY created_at DESC LIMIT 5;
-- SELECT id, username, created_at FROM users ORDER BY created_at DESC LIMIT 5;
-- SELECT id, created_at FROM deposits ORDER BY created_at DESC LIMIT 5;
