#!/bin/sh
set -eu

: "${POSTGRES_HOST:=postgres}"
: "${POSTGRES_PORT:=5432}"
: "${POSTGRES_USER:?POSTGRES_USER is required}"
: "${POSTGRES_PASSWORD:?POSTGRES_PASSWORD is required}"
: "${AIM_DATABASE:?AIM_DATABASE is required}"
: "${AIM_MIGRATIONS_DIR:=/migrations}"

export PGPASSWORD="${PGPASSWORD:-$POSTGRES_PASSWORD}"

case "$AIM_DATABASE" in
  ""|*[!A-Za-z0-9_]*)
    echo "invalid AIM_DATABASE: $AIM_DATABASE" >&2
    exit 2
    ;;
esac

psql_cmd() {
  psql -v ON_ERROR_STOP=1 -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" "$@"
}

if ! psql_cmd -d postgres -tc "SELECT 1 FROM pg_database WHERE datname = '$AIM_DATABASE'" | grep -q 1; then
  echo "creating database $AIM_DATABASE"
  createdb -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" "$AIM_DATABASE"
fi

set -- "$AIM_MIGRATIONS_DIR"/*.sql
if [ ! -e "$1" ]; then
  echo "no migration files found in $AIM_MIGRATIONS_DIR"
  exit 0
fi

for file in "$@"; do
  echo "applying $AIM_DATABASE migration: $(basename "$file")"
  psql_cmd -d "$AIM_DATABASE" -f "$file"
done
