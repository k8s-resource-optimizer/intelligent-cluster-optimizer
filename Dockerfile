# Build stage
FROM --platform=$BUILDPLATFORM mirror.gcr.io/library/golang:1.26-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go modules first for layer caching
COPY go.mod go.sum ./
COPY vendor/ vendor/

# Copy source code
COPY . .

# Build controller binary
# -mod=vendor uses the local vendor dir so no outbound network is required
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -mod=vendor \
    -ldflags="-w -s" \
    -o /app/bin/controller \
    ./cmd/controller

# Final stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/controller /controller

USER nonroot:nonroot

# 8080 = Prometheus metrics   8090 = GUI REST API
EXPOSE 8080 8090

ENTRYPOINT ["/controller"]
