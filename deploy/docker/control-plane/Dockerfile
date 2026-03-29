FROM golang:1.26.1

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN go build -o /usr/local/bin/control-plane ./cmd/control-plane

EXPOSE 8080

CMD ["control-plane"]
