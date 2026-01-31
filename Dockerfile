# Multi-stage Dockerfile for i2plan mesh VPN
# Builds statically-linked binary and creates minimal runtime image

# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /build

# Copy go module files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version injection
ARG VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_TIME=unknown

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -extldflags '-static' \
      -X github.com/go-i2p/wireguard/version.Version=${VERSION} \
      -X github.com/go-i2p/wireguard/version.GitCommit=${GIT_COMMIT} \
      -X github.com/go-i2p/wireguard/version.BuildTime=${BUILD_TIME}" \
    -o i2plan ./cmd/i2plan

# Verify static binary
RUN ldd i2plan 2>&1 | grep -q "not a dynamic executable" || \
    (echo "Binary is not static" && exit 1)

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    iptables \
    ip6tables \
    wireguard-tools

# Create non-root user (though i2plan needs NET_ADMIN capability)
RUN addgroup -g 1000 i2plan && \
    adduser -D -u 1000 -G i2plan i2plan

# Copy binary from builder
COPY --from=builder /build/i2plan /usr/local/bin/i2plan

# Copy example config
COPY config.example.toml /etc/i2plan/config.example.toml

# Create data directory
RUN mkdir -p /var/lib/i2plan && \
    chown i2plan:i2plan /var/lib/i2plan

# Set working directory
WORKDIR /var/lib/i2plan

# Labels for metadata
LABEL org.opencontainers.image.title="i2plan"
LABEL org.opencontainers.image.description="WireGuard mesh VPN over I2P"
LABEL org.opencontainers.image.url="https://github.com/go-i2p/wireguard"
LABEL org.opencontainers.image.source="https://github.com/go-i2p/wireguard"
LABEL org.opencontainers.image.vendor="go-i2p"
LABEL org.opencontainers.image.licenses="See LICENSE file"

# Run as root (required for network interface creation)
# In production, use --cap-add=NET_ADMIN instead of root where possible
USER root

ENTRYPOINT ["/usr/local/bin/i2plan"]
CMD []
