path "keys/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "identity/oidc/client/libops-api" {
  capabilities = [ "read" ]
}

path "identity/entity" {
  capabilities = [ "create", "update" ]
}

path "identity/entity/id/*" {
  capabilities = [ "create", "read", "update", "delete" ]
}

path "identity/entity-alias" {
  capabilities = [ "create", "update" ]
}

path "identity/entity-alias/id/*" {
  capabilities = [ "read", "update", "delete", "list" ]
}

path "auth/userpass/users/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/token/create/entity-token" {
  capabilities = [ "create", "update"]
}

path "sys/auth" {
  capabilities = ["read", "list"]
}

path "secret/libops-api" {
  capabilities = ["read", "list"]
}

path "secret/libops-api/*" {
  capabilities = ["read"]
}
