FROM golang:1.22-alpine AS builder

WORKDIR /src
COPY go.mod ./
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/mole-server ./cmd/server
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/mole-server /mole-server

EXPOSE 9000
EXPOSE 9001
EXPOSE 10000-10100

ENTRYPOINT ["/mole-server"]
