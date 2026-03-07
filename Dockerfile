FROM golang:1.25.6 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/rhea-api

FROM debian:bookworm-slim

WORKDIR /app
COPY --from=builder /app/server /app/server

ENV PORT=10000
EXPOSE 10000

CMD ["/app/server"]
