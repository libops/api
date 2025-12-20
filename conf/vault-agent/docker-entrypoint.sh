#!/usr/bin/env bash

set -eou pipefail

SLEEP_TIME=300
JWT_SINK_FILE=/vault/secrets/GOOGLE_ACCESS_TOKEN
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
  if [ "$ENVIRONMENT" = "local" ]; then
    return 0
  fi
  PRIVATE_KEY=$(jq -er .private_key /run/secrets/GOOGLE_APPLICATION_CREDENTIALS)
  CLIENT_EMAIL=$(jq -er .client_email /run/secrets/GOOGLE_APPLICATION_CREDENTIALS)
  HEADER=$(echo -n '{"alg":"RS256","typ":"JWT"}' | openssl base64 -e | tr -d '=' | tr '/+' '_-' | tr -d '\n')
  NOW=$(date +%s)
  TTL=$(( SLEEP_TIME * 3 ))
  EXP=$((NOW + TTL))
  CLAIM=$(cat <<EOF
{
  "iss": "$CLIENT_EMAIL",
  "scope": "https://www.googleapis.com/auth/cloud-platform",
  "aud": "https://oauth2.googleapis.com/token",
  "exp": $EXP,
  "iat": $NOW
}
EOF
)
  CLAIM_BASE64=$(echo -n "$CLAIM" | openssl base64 -e | tr -d '=' | tr '/+' '_-' | tr -d '\n')
  SIGNATURE_INPUT="$HEADER.$CLAIM_BASE64"
  SIGNATURE=$(echo -n "$SIGNATURE_INPUT" | openssl dgst -sha256 -sign <(echo "$PRIVATE_KEY") | openssl base64 -e | tr -d '=' | tr '/+' '_-' | tr -d '\n')
  JWT="$HEADER.$CLAIM_BASE64.$SIGNATURE"

  # Get both access_token (for GCP APIs like Pub/Sub) and id_token (for Vault auth)
  # Note: We need to request target_audience to get an id_token
  VAULT_ADDR=${VAULT_ADDR:-http://vault:8200}
  RESPONSE=$(curl -s -X POST https://oauth2.googleapis.com/token \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer&assertion=$JWT")

  # For Vault JWT auth, we need an id_token, not an access_token
  # Request an id_token with Vault's configured audience
  ID_TOKEN_RESPONSE=$(curl -s -X POST https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/"$CLIENT_EMAIL":generateIdToken \
    -H "Authorization: Bearer $(echo "$RESPONSE" | jq -r '.access_token')" \
    -H "Content-Type: application/json" \
    -d "{\"audience\":\"https://vault.libops.io\",\"includeEmail\":true}")

  ID_TOKEN=$(echo "$ID_TOKEN_RESPONSE" | jq -er '.token')
  if [ "$ID_TOKEN" != "" ] && [ "$ID_TOKEN" != "null" ]; then
    echo "$ID_TOKEN" > "$JWT_SINK_FILE"
    return 0
  fi

  echo "Failed to get id_token from GCP: $ID_TOKEN_RESPONSE" >&2
  return 1
}

while true; do
  MAX_RETRIES=10
  SLEEP_INCREMENT=5
  RETRIES=0

  # populate the JWT from google using machine's credentials
  while true; do
    if generate_jwt; then
      break
    fi

    RETRIES=$((RETRIES + 1))
    if [ "$RETRIES" -ge "$MAX_RETRIES" ]; then
        echo "FAILURE: Failed to get JWT after $MAX_RETRIES attempts." >&2
        exit 1
    fi

    BASE_DELAY=$(( SLEEP_INCREMENT * (1 << (RETRIES - 1)) ))
    JWT_SLEEP=$(( BASE_DELAY < 120 ? BASE_DELAY : 120 ))
    echo "generate_jwt failed (Attempt $RETRIES/$MAX_RETRIES). Retrying in $JWT_SLEEP seconds..." >&2
    sleep "$JWT_SLEEP"
  done

  # now that we have a JWT we can auth to vault
  vault agent -config="/etc/vault/agents/${ENVIRONMENT}.hcl" &
  PID=$!
  wait "$PID"
  sleep "$SLEEP_TIME"
done
