# Build Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache make git

# Copy go.mod/go.sum first
COPY services/sentinel-proxy-go/go.mod services/sentinel-proxy-go/go.sum ./
RUN go mod download  

# Copy source code
COPY services/sentinel-proxy-go .

# Build the binary
# CGO_ENABLED=0 for static binary, -ldflags for smaller size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/bin/sentinel-proxy-go main.go

# Final Stage
FROM alpine:3.19

WORKDIR /app

# Install certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1001 -S appuser && \
    adduser -u 1001 -S appuser -G appuser

# Copy binary from builder
COPY --from=builder /app/bin/sentinel-proxy-go .

# Copy public assets (dashboard)
COPY --from=builder /app/public ./public

# Change ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Run
CMD ["./sentinel-proxy-go"]
