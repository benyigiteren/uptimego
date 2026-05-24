# Stage 1: Build the Go application
FROM golang:alpine AS builder

# Install build dependencies
RUN apk add --no-cache git

WORKDIR /app

# Copy dependency manifests
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire codebase
COPY . .

# Compile statically with CGO disabled (pure Go SQLite is used)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o uptimego .

# Stage 2: Create the final minimal production image
FROM alpine:latest

# Install CA Certificates (crucial for checking HTTPS/SSL sites) and ping tool
RUN apk add --no-cache ca-certificates tzdata iputils

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/uptimego /app/uptimego

# Expose port
EXPOSE 8086

# Run the app
ENTRYPOINT ["/app/uptimego", "-port", "8086", "-db", "/app/data/uptimego.db"]
