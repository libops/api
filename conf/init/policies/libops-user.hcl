path "identity/oidc/token/libops-api" {
  capabilities = ["read", "update"]
}

# Allow users to manage their own API keys using ACL templating
# The account_uuid metadata is set on the entity in lowercase no-dashes format
path "keys/{{identity.entity.metadata.account_uuid}}/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
