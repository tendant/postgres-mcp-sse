FROM golang:1.24 AS builder

WORKDIR /app
COPY . .

RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o mcp-server main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/mcp-server .
CMD ["./mcp-server"]
