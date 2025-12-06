# Build Stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies (if any needed, e.g. git, make)
RUN apk add --no-cache make git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/sentinel-proxy-go main.go

# Final Stage
FROM alpine:3.19

WORKDIR /app

# Install certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Copy binary from builder
COPY --from=builder /app/bin/sentinel-proxy-go .

# Copy public assets (dashboard)
COPY --from=builder /app/public ./public

# Expose port
EXPOSE 8080

# Run
CMD ["./sentinel-proxy-go"]
