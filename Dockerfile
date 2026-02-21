# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o context-service ./cmd/context

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install CA certificates for HTTPS
RUN apk add --no-cache ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/context-service .

# Run as non-root user
RUN adduser -D -g '' appuser
USER appuser

ENTRYPOINT ["./context-service"]
