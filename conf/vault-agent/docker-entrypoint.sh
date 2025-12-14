#!/usr/bin/env bash

set -eou pipefail

cleanup() {
  echo "Caught SIGTERM or SIGINT. Shutting down Vault Agent..."
  if [ -n "$PID" ] && ps -p "$PID" > /dev/null; then
    kill -SIGTERM "$PID"
    wait "$PID" || echo "Vault Agent terminated."
  fi
  exit 0
}

trap cleanup SIGTERM SIGINT

generate_jwt() {
  PRIVATE_KEY=$(jq -er .private_key /run/secrets/GOOGLE_APPLICATION_CREDENTIALS)
  CLIENT_EMAIL=$(jq -er .client_email /run/secrets/GOOGLE_APPLICATION_CREDENTIALS)
  HEADER=$(echo -n '{"alg":"RS256","typ":"JWT"}' | openssl base64 -e | tr -d '=' | tr '/+' '_-' | tr -d '\n')
  NOW=$(date +%s)
  EXP=$((NOW + 300))
  CLAIM=$(cat <<EOF
{
  "iss": "$CLIENT_EMAIL",
  "aud": "https://oauth2.googleapis.com/token",
  "target_audience": "https://vault.libops.io",
  "exp": $EXP,
  "iat": $NOW
}
EOF
)
  CLAIM_BASE64=$(echo -n "$CLAIM" | openssl base64 -e | tr -d '=' | tr '/+' '_-' | tr -d '\n')
  SIGNATURE_INPUT="$HEADER.$CLAIM_BASE64"
  SIGNATURE=$(echo -n "$SIGNATURE_INPUT" | openssl dgst -sha256 -sign <(echo "$PRIVATE_KEY") | openssl base64 -e | tr -d '=' | tr '/+' '_-' | tr -d '\n')
  JWT="$HEADER.$CLAIM_BASE64.$SIGNATURE"
  curl -s -X POST https://oauth2.googleapis.com/token \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer&assertion=$JWT" \
    | jq -er '.id_token' > /run/secrets/GOOGLE_ACCESS_TOKEN
}

while true; do
  generate_jwt
  vault agent -config="/etc/vault/agents/${ENVIRONMENT}.hcl" &
  PID=$!
  wait "$PID"
  sleep 600
done
