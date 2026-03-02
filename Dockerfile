# Build stage
FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gotunnel ./cmd/gotunnel

# Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /gotunnel /usr/local/bin/gotunnel

EXPOSE 80 443 9090

ENTRYPOINT ["gotunnel"]
CMD ["server", "--config", "/etc/gotunnel/config.yml"]
