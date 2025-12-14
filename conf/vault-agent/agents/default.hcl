exit_after_auth = true
pid_file = "/tmp/vault-agent.pid"

vault {
  address = "http://vault:8200"
  tls_skip_verify = true
}

auto_auth {
  method {
    type = "jwt"
    config = {
      path = "/run/secrets/GOOGLE_ACCESS_TOKEN"
      role = "libops-api"
    }
  }
  sink "file" {
    config = {
      path = "/vault/secrets/VAULT_TOKEN"
      owner = 100
      group = 1000
    }
  }
}

template {
  source      = "/etc/vault/templates/oidc-client-id.ctmpl"
  destination = "/vault/secrets/OIDC_CLIENT_ID"
}

template {
  source      = "/etc/vault/templates/oidc-client-secret.ctmpl"
  destination = "/vault/secrets/OIDC_CLIENT_SECRET"
}
