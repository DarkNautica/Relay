# ─────────────────────────────────────────────
# Relay Server — Dockerfile
# Multi-stage build: compiles Go, produces a minimal final image
# ─────────────────────────────────────────────

# Stage 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download dependencies first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o relay ./main.go

# Stage 2: Final image
FROM alpine:3.19

# Install CA certificates for HTTPS outbound (webhooks, etc.)
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /relay

# Copy the compiled binary
COPY --from=builder /app/relay .

# Relay runs on port 6001 by default
EXPOSE 6001

# Run as non-root user for security
RUN addgroup -S relay && adduser -S relay -G relay
USER relay

CMD ["./relay"]
