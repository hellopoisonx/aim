-- provision_bot.sql
--
-- Stamps a single bot identity into AIM. Designed to be invoked twice
-- against two separate databases via psql -v variables:
--
--   psql aim_auth  -v stage=auth  -f provision_bot.sql ...
--   psql aim_logic -v stage=logic -f provision_bot.sql ...
--
-- :stage controls which half of the script runs. Other variables:
--
--   bot_user_id          int8        — the bot's user_id (Snowflake)
--   bot_email            text        — placeholder email (NOT used for login)
--   bot_nickname         text        — nickname displayed in group rosters
--   token_id             int8        — Snowflake id for the bot_tokens row
--   token_name           text        — operator-friendly label
--   token_scopes_csv     text        — e.g. "messages:send"
--   token_hash           text        — sha256 hex of the plaintext token
--   placeholder_password text        — non-bcrypt placeholder, login disabled
--   conversation_ids_csv text        — comma-separated, may be empty
--
-- The script is idempotent: re-running with the same bot_user_id will
-- leave the existing rows in place. To rotate a token, insert a new
-- bot_tokens row with a different token_id.

\set ON_ERROR_STOP on

-- AUTH stage --------------------------------------------------------------

\if :{?stage}
\else
\echo ERROR: psql variable :stage must be set to 'auth' or 'logic'
\quit 1
\endif

\if :{?bot_user_id}
\else
\echo ERROR: psql variable :bot_user_id must be set
\quit 1
\endif

DO $$
DECLARE
    s text := :'stage';
BEGIN
    IF s NOT IN ('auth', 'logic') THEN
        RAISE EXCEPTION 'unknown stage: %', s;
    END IF;
END$$;

-- AUTH branch -----------------------------------------------------------------
INSERT INTO user_credentials (id, email, password_hash, status)
SELECT :bot_user_id, :'bot_email', :'placeholder_password', 1
WHERE :'stage' = 'auth'
ON CONFLICT (id) DO NOTHING;

-- LOGIC branch ----------------------------------------------------------------
INSERT INTO user_info (id, email, nickname, avatar, status, user_type)
SELECT :bot_user_id, :'bot_email', :'bot_nickname', 'https://implement.me', 1, 'bot'
WHERE :'stage' = 'logic'
ON CONFLICT (id) DO UPDATE SET
    user_type = EXCLUDED.user_type,
    nickname = EXCLUDED.nickname,
    updated_at = NOW();

-- Token row (logic stage only) ------------------------------------------------
INSERT INTO bot_tokens (id, bot_user_id, token_hash, name, scopes)
SELECT :token_id,
       :bot_user_id,
       :'token_hash',
       :'token_name',
       string_to_array(:'token_scopes_csv', ',')
WHERE :'stage' = 'logic'
ON CONFLICT (token_hash) DO NOTHING;

-- Membership (logic stage only) ----------------------------------------------
WITH ids AS (
    SELECT NULLIF(trim(s), '')::bigint AS conv_id
    FROM regexp_split_to_table(:'conversation_ids_csv', ',') AS s
)
INSERT INTO conversation_members (conversation_id, user_id, role)
SELECT i.conv_id, :bot_user_id, 'member'
FROM ids i
WHERE :'stage' = 'logic'
  AND i.conv_id IS NOT NULL
ON CONFLICT (conversation_id, user_id) DO NOTHING;
