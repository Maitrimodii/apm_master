# syntax=docker/dockerfile:1

# ------------------------------------------------------------------------------
# 1. Base Stage: Go toolchain & dependencies
# ------------------------------------------------------------------------------
FROM golang:1.24-alpine AS base

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# ------------------------------------------------------------------------------
# 2. Test Stage: Automated testing in container
# ------------------------------------------------------------------------------
FROM base AS test

CMD ["go", "test", "-v", "./..."]

# ------------------------------------------------------------------------------
# 3. Builder Stage: Compile static binaries
# ------------------------------------------------------------------------------
FROM base AS builder

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/apm ./cmd/apm
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/scheduler ./cmd/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/migrate ./cmd/migrate

# ------------------------------------------------------------------------------
# 4. Production Stage: Minimal, secure scratch/alpine container
# ------------------------------------------------------------------------------
FROM alpine:3.21 AS production

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S appgroup && adduser -S appuser -G appgroup

COPY --from=builder /bin/apm /bin/scheduler /bin/migrate ./
COPY --from=builder /app/migrations ./migrations

RUN chown -R appuser:appgroup /app

USER appuser

EXPOSE 8080

CMD ["./apm"]
