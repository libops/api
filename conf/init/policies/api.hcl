path "keys/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "identity/oidc/client/libops-api" {
  capabilities = [ "read" ]
}

path "identity/entity/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "identity/entity-alias/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/userpass/users/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/token/create" {
  capabilities = ["update"]
}

path "secret/libops-api" {
  capabilities = ["read", "list"]
}

path "secret/libops-api/*" {
  capabilities = ["read"]
}
