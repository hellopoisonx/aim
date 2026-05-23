#!/usr/bin/env bash
# provision_bot.sh — one-shot Bot identity bootstrap.
#
# Writes both halves (auth.user_credentials + logic.user_info / bot_tokens
# / conversation_members) inside one transaction per database. Prints the
# plaintext token exactly once on stdout.
#
# Requires: bash 4+, psql, openssl, sha256sum (or shasum on macOS).
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: provision_bot.sh [options]

Required:
  --bot-user-id <int>          Bot's user_id (Snowflake-friendly int64)
  --bot-email <email>          Placeholder email (e.g. broadcast@bots.aim)
  --bot-nickname <name>        Nickname displayed in group rosters
  --auth-dsn <postgres-dsn>    DSN for the aim_auth database
  --logic-dsn <postgres-dsn>   DSN for the aim_logic database

Optional:
  --conversation-ids <csv>     Comma-separated group IDs the bot should join
  --token-name <name>          Token label (default: "default")
  --token-scopes <csv>         Comma-separated action grants
                               (default: "bot.message.send,bot.conversation.list,bot.self.read")
  --token-id <int>             Snowflake id for the bot_tokens row
                               (default: $(($(date +%s%3N))))
  --plaintext <token>          Use this plaintext instead of generating one
                               (advanced, e.g. when re-running idempotently)
  -h, --help                   Show this help

Example:
  ./provision_bot.sh \\
    --bot-user-id 9000000001 \\
    --bot-email broadcast@bots.aim \\
    --bot-nickname broadcast-bot \\
    --conversation-ids "1,2,3" \\
    --auth-dsn  "postgresql://user:password@localhost:5432/aim_auth" \\
    --logic-dsn "postgresql://user:password@localhost:5432/aim_logic"
EOF
}

bot_user_id=""
bot_email=""
bot_nickname=""
conversation_ids=""
token_name="default"
token_scopes="bot.message.send,bot.conversation.list,bot.self.read"
token_id=""
plaintext=""
auth_dsn=""
logic_dsn=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bot-user-id)        bot_user_id="$2"; shift 2 ;;
    --bot-email)          bot_email="$2"; shift 2 ;;
    --bot-nickname)       bot_nickname="$2"; shift 2 ;;
    --conversation-ids)   conversation_ids="$2"; shift 2 ;;
    --token-name)         token_name="$2"; shift 2 ;;
    --token-scopes)       token_scopes="$2"; shift 2 ;;
    --token-id)           token_id="$2"; shift 2 ;;
    --plaintext)          plaintext="$2"; shift 2 ;;
    --auth-dsn)           auth_dsn="$2"; shift 2 ;;
    --logic-dsn)          logic_dsn="$2"; shift 2 ;;
    -h|--help)            usage; exit 0 ;;
    *)                    echo "unknown flag: $1" >&2; usage; exit 1 ;;
  esac
done

require() {
  if [[ -z "${!1}" ]]; then
    echo "missing required flag: --${1//_/-}" >&2
    exit 1
  fi
}
require bot_user_id
require bot_email
require bot_nickname
require auth_dsn
require logic_dsn

IFS=',' read -ra _action_parts <<< "$token_scopes"
for raw_action in "${_action_parts[@]}"; do
  action="$(echo "$raw_action" | xargs)"
  if [[ -z "$action" ]]; then
    continue
  fi
  if [[ "$action" != "*" && ! "$action" =~ ^bot\.[a-z0-9_]+(\.[a-z0-9_]+)*(\.\*)?$ ]]; then
    echo "invalid bot action grant: ${action}" >&2
    echo "use action names like bot.message.send; old scopes such as messages:send are not supported" >&2
    exit 1
  fi
done

if [[ -z "$plaintext" ]]; then
  random_hex="$(openssl rand -hex 32)"
  plaintext="aim_bot_${random_hex}"
fi

# SHA-256 portable across linux/macos.
if command -v sha256sum >/dev/null 2>&1; then
  token_hash="$(printf '%s' "$plaintext" | sha256sum | awk '{print $1}')"
else
  token_hash="$(printf '%s' "$plaintext" | shasum -a 256 | awk '{print $1}')"
fi

if [[ -z "$token_id" ]]; then
  token_id="$(date +%s%3N)"
fi

# A non-bcrypt placeholder so auth.Login refuses any password attempt.
placeholder_password="DISABLED::bot::$(date -u +%Y%m%dT%H%M%SZ)"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
sql_file="${script_dir}/provision_bot.sql"

run_sql() {
  local dsn="$1"
  local stage="$2"
  psql "$dsn" \
    -v ON_ERROR_STOP=on \
    -v "stage=${stage}" \
    -v "bot_user_id=${bot_user_id}" \
    -v "bot_email=${bot_email}" \
    -v "bot_nickname=${bot_nickname}" \
    -v "token_id=${token_id}" \
    -v "token_name=${token_name}" \
    -v "token_scopes_csv=${token_scopes}" \
    -v "token_hash=${token_hash}" \
    -v "placeholder_password=${placeholder_password}" \
    -v "conversation_ids_csv=${conversation_ids}" \
    -f "$sql_file"
}

run_sql "$auth_dsn" auth >/dev/null
run_sql "$logic_dsn" logic >/dev/null

cat <<EOF

=== AIM Bot Provisioned ===
bot_user_id : ${bot_user_id}
nickname    : ${bot_nickname}
token_id    : ${token_id}
actions     : ${token_scopes}
groups      : ${conversation_ids:-<none>}
plaintext   : ${plaintext}

WARNING: the plaintext token above is shown ONCE and is unrecoverable.
Store it now in your secret manager.
EOF
