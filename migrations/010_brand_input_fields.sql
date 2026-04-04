-- Add dynamic input fields configuration to brand_settings
-- This allows admins to configure game-specific input fields (User ID, Zone ID, Server, etc.)
-- without requiring code changes.

-- input_fields: JSONB array defining the input fields for each brand
-- Example: [{"key":"user_id","type":"text","label":"User ID","placeholder":"Masukkan User ID","required":true},
--           {"key":"server","type":"select","label":"Server","placeholder":"Pilih Server","required":true,"options":["Asia","America"]}]
ALTER TABLE brand_settings
ADD COLUMN IF NOT EXISTS input_fields JSONB DEFAULT '[]'::jsonb;

-- input_separator: Character used to join multiple input field values into customer_no
-- Examples: "" (no separator), "|", "#", "/", " " (space)
ALTER TABLE brand_settings
ADD COLUMN IF NOT EXISTS input_separator VARCHAR(10) DEFAULT '';
