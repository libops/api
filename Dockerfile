FROM ghcr.io/libops/go1.25:main@sha256:f43c9b34f888d2ac53e87c8e061554f826b8eb580863d7b21fd787b6f0378f8f AS builder

SHELL ["/bin/ash", "-o", "pipefail", "-ex", "-c"]

WORKDIR /app

COPY go.mod go.sum ./
COPY proto/ ./proto/
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY *.go ./
COPY internal/ ./internal/
COPY db/ ./db/

RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/binary .

FROM node:22-alpine AS frontend

WORKDIR /app

# Copy package files and install dependencies
COPY web/package.json web/package-lock.json ./
RUN npm ci

# Copy source files and build configuration
COPY web/src/ ./src/
COPY web/tsconfig.json web/vite.config.ts web/vite-plugin-proto-resolver.ts ./

# Build the frontend bundle
RUN npm run build

FROM ubuntu:24.04 AS tailwind

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /app

RUN apt-get update && apt-get install curl --yes && \
    curl -sL \
    https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.18/tailwindcss-linux-x64 \
    -o /app/tailwindcss && \
    chmod +x /app/tailwindcss

COPY web/ ./web/
# Copy the bundled JavaScript from frontend stage
COPY --from=frontend /app/static/js/main.bundle.js ./web/static/js/main.bundle.js

RUN /app/tailwindcss \
    -i ./web/static/css/input.css \
    -o ./web/static/css/output.css \
    --content './web/templates/**/*.html,./web/src/**/*.ts' \
    --minify


FROM ghcr.io/libops/go1.25:main@sha256:f43c9b34f888d2ac53e87c8e061554f826b8eb580863d7b21fd787b6f0378f8f

WORKDIR /app
COPY --from=builder /app/binary /app/binary
COPY web/ ./web/
COPY --from=tailwind /app/web/static/css/output.css ./web/static/css/output.css
COPY --from=frontend /app/static/js/main.bundle.js ./web/static/js/main.bundle.js

COPY openapi/ ./openapi/

EXPOSE 8080

HEALTHCHECK CMD /bin/bash -c 'curl -sf http://localhost:8080/health'
