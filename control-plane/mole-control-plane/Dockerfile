FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mole-control-plane ./cmd/api

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /out/mole-control-plane /app/mole-control-plane

EXPOSE 8080

ENTRYPOINT ["/app/mole-control-plane"]
