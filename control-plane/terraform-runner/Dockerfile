# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -o terraform-runner .

# Runtime stage
FROM hashicorp/terraform:1.9

# Install curl for API calls
RUN apk add --no-cache curl ca-certificates

# Copy terraform-runner binary from builder
COPY --from=builder /build/terraform-runner /usr/local/bin/terraform-runner

# Create workspace directory
RUN mkdir -p /workspace/terraform

# Set working directory
WORKDIR /workspace

# Run as non-root user
USER 1000:1000

# Entry point
ENTRYPOINT ["/usr/local/bin/terraform-runner"]
