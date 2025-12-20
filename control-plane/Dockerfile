FROM ghcr.io/libops/base:main AS builder

WORKDIR /app

COPY db/go.mod ./db/
COPY proto/go.mod proto/go.sum ./proto/
COPY control-plane/go.mod control-plane/go.sum ./control-plane/

WORKDIR /app/proto
RUN go mod download
WORKDIR /app/control-plane
RUN go mod download

WORKDIR /app
COPY db/ ./db/
COPY control-plane/ ./control-plane/
COPY proto/ ./proto/

# Build control-plane binaries
WORKDIR /app/control-plane
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/event-router ./cmd/event-router

FROM ghcr.io/libops/base:main

WORKDIR /app

COPY --from=builder /bin/event-router /app/event-router

USER goapp

ENTRYPOINT ["/app/event-router"]
