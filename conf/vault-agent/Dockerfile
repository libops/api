FROM hashicorp/vault:1.21@sha256:f4e2687b72858a9e2160c344c9fa1ef74c07f21a89a8c00534ab64d3f187b927

RUN apk add bash curl jq openssl yq --no-cache \
    && mkdir -p /etc/vault

COPY agents /etc/vault/agents
COPY templates /etc/vault/templates

COPY docker-entrypoint.sh /

ENV \
    ENVIRONMENT=default

ENTRYPOINT [ "/docker-entrypoint.sh" ]
HEALTHCHECK NONE
