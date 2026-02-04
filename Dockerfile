# Build Stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git just in case dependencies need it
RUN apk add --no-cache git

# Copy dependencies first for caching layers
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0 creates a statically linked binary
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Run Stage (Production)
FROM alpine:latest

WORKDIR /app

# Install CA certificates (for HTTPS) and Timezone data
RUN apk add --no-cache ca-certificates tzdata

# Set Timezone to Jakarta (matching your location)
ENV TZ=Asia/Jakarta

# Copy binary from builder
COPY --from=builder /app/main .

# Copy migrations folder (in case you need to run migration tools later)
COPY --from=builder /app/migrations ./migrations

# Expose the API port
EXPOSE 8080

# Run the binary
CMD ["./main"]
