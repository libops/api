#!/bin/sh
set -e

if [ -n "$MARIADB_PASSWORD_FILE" ] && [ -f "$MARIADB_PASSWORD_FILE" ]; then
    DB_PASS=$(cat "$MARIADB_PASSWORD_FILE")
    # If DATABASE_URL is set but contains the placeholder password or needs injection
    # We'll just construct it.
    # The default in docker-compose.ci.yaml is: libops:libops-test-pass@tcp(mariadb:3306)/libops?parseTime=true
    # We replace libops-test-pass with the real password.
    # Or simpler: just reconstruct it since we know the structure for tests.
    export DATABASE_URL="libops:${DB_PASS}@tcp(mariadb:3306)/libops?parseTime=true"
fi

exec /app/test-runner "$@"
