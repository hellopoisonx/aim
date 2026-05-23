ALTER TABLE user_credentials ADD COLUMN IF NOT EXISTS name VARCHAR(255) NOT NULL DEFAULT '';

UPDATE user_credentials
SET name = COALESCE(NULLIF(TRIM(name), ''), split_part(email, '@', 1))
WHERE TRIM(name) = '';

ALTER TABLE user_credentials ALTER COLUMN name DROP DEFAULT;
