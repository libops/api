default_max_request_duration = "90s"
disable_clustering           = true
disable_mlock                = true
ui                           = true
api_addr                     = "http://vault.libops.io"
log_requests_level           = "info"

listener "tcp" {
  address     = "0.0.0.0:8200"
  tls_disable = "true"
}

seal "gcpckms" {
  key_ring   = "vault-server"
  crypto_key = "vault"
  region     = "global"
  project    = "libops-api"
}

storage "file" {
  path = "/vault/data"
}
