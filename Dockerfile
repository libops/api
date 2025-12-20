FROM ghcr.io/libops/base:main AS builder

SHELL ["/bin/ash", "-o", "pipefail", "-ex", "-c"]

WORKDIR /app

COPY go.mod go.sum ./
COPY proto/ ./proto/
COPY db/ ./db/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY *.go ./
COPY internal/ ./internal/

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/binary .

FROM node:22-alpine AS frontend

WORKDIR /app

COPY web/package.json web/package-lock.json ./
RUN npm ci

COPY web/src/ ./src/
COPY web/tsconfig.json web/vite.config.ts web/vite-plugin-proto-resolver.ts ./
RUN npm run build

FROM ubuntu:24.04 AS tailwind

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /app

ARG TARGETARCH
ARG ARCH_SUFFIX=${TARGETARCH}
RUN if [ "${TARGETARCH}" = "amd64" ]; then ARCH_SUFFIX="x64"; fi && \
    apt-get update && apt-get install curl --yes && \
    curl -sL \
    "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.18/tailwindcss-linux-${ARCH_SUFFIX}" \
    -o /app/tailwindcss && \
    chmod +x /app/tailwindcss

COPY web/ ./web/
COPY --from=frontend /app/static/js/main.bundle.js ./web/static/js/main.bundle.js

RUN /app/tailwindcss \
    -i ./web/static/css/input.css \
    -o ./web/static/css/output.css \
    --content './web/templates/**/*.html,./web/src/**/*.ts' \
    --minify


FROM ghcr.io/libops/base:main

WORKDIR /app
COPY --from=builder /app/binary /app/binary
COPY web/ ./web/
COPY --from=tailwind /app/web/static/css/output.css ./web/static/css/output.css
COPY --from=frontend /app/static/js/main.bundle.js ./web/static/js/main.bundle.js

COPY openapi/ ./openapi/

RUN chown -R goapp . && \
    find . -type d -exec chmod 550 {} \; && \
    find . -type f -exec chmod 440 {} \; && \
    chmod +x /app/binary

USER goapp

EXPOSE 8080

ENTRYPOINT [ "/app/binary" ]

HEALTHCHECK CMD /bin/bash -c 'curl -sf http://localhost:8080/health || exit 1'
