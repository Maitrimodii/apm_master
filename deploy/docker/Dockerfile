# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o apm ./cmd/apm
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o scheduler ./cmd/scheduler

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binaries and migrations
COPY --from=builder /app/apm .
COPY --from=builder /app/scheduler .
COPY --from=builder /app/migrations ./migrations

# Expose the API port
EXPOSE 8080

# Command to run the application
CMD ["./apm"]
