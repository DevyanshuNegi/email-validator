# Multi-stage build for Go worker
# Using Go 1.25 to match go.mod requirements
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY *.go ./

# Build the binary
# CGO_ENABLED=0 creates a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o validator-worker .

# Final stage - minimal Alpine image
FROM alpine:latest

# Install CA certificates for HTTPS/TLS connections
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/validator-worker .

# Create a non-root user for security
RUN addgroup -S worker && adduser -S worker -G worker
RUN chown worker:worker /root/validator-worker

USER worker

# Run the worker
CMD ["./validator-worker"]
