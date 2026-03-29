FROM golang:1.26.1 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 go build -o /usr/local/bin/control-plane ./cmd/control-plane

FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates docker.io \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/control-plane /usr/local/bin/control-plane

WORKDIR /app
COPY deploy/docker/managed-user/image.lock deploy/docker/managed-user/image.lock

EXPOSE 8080

CMD ["control-plane"]
