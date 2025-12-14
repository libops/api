path "secret/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "identity/*" {
  capabilities = [ "create", "read", "update", "delete", "list" ]
}

path "auth/token/create" {
  capabilities = ["update"]
}

path "auth/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "sys/policies/acl/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "sys/auth/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}

path "sys/mounts/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
