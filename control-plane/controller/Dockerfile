FROM ghcr.io/libops/base:main AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o controller .

FROM ghcr.io/libops/base:main

WORKDIR /app

COPY --from=builder /build/controller .

RUN mkdir -p /tmp/triggers && chmod 755 /tmp/triggers

EXPOSE 8080

CMD ["./controller"]
