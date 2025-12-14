FROM hashicorp/vault:1.21

RUN apk add bash yq --no-cache

WORKDIR /init

COPY policies ./policies
COPY docker-entrypoint.sh /init/

ENTRYPOINT [ "/init/docker-entrypoint.sh" ]
