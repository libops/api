#!/usr/bin/env bash

set -eou pipefail

echo 'Importing seed data...'
# Sort files to ensure consistent ordering (00-functions.sql first)

DB_PASS="libops-test-pass"
if [ -n "${MARIADB_PASSWORD_FILE:-}" ]; then
    DB_PASS=$(cat "$MARIADB_PASSWORD_FILE")
fi

for f in /testdata/*.sql; do
    echo "Importing $f..."
    mariadb -h mariadb -u libops -p"$DB_PASS" libops < "$f"
done
echo 'Seed data imported successfully!'
