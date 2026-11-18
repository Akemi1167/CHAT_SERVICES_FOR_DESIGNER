# ---------- build ----------
FROM golang:1.26-alpine AS builder

WORKDIR /src
RUN apk add --no-cache git ca-certificates tzdata

# Cache layer cho dependency.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---------- runtime ----------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata curl \
 && adduser -D -u 10001 app
WORKDIR /app

COPY --from=builder /out/server /app/server
COPY --from=builder /src/migrations /app/migrations

USER app
EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
  CMD curl -fsS http://localhost:8080/healthz || exit 1

ENTRYPOINT ["/app/server"]
