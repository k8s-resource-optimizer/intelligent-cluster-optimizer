# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go modules first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build controller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/controller \
    ./cmd/controller

# Final stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /app/bin/controller /controller

USER nonroot:nonroot

EXPOSE 8080 8081

ENTRYPOINT ["/controller"]
