FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY manager/go.mod manager/go.sum ./
RUN go mod download

COPY manager/ ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /demimine ./cmd/server

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /demimine /usr/local/bin/demimine

RUN mkdir -p /data /servers /proxies /backups /java

WORKDIR /app

EXPOSE 8080

CMD ["demimine"]
