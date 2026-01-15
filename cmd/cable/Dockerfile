
FROM golang:latest AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o main cmd/cable/*.go

FROM alpine:latest
COPY --from=builder /app/main /
CMD ["/main"]
EXPOSE 1881 1882 1883 1884 1885/udp 1886/udp 6060 8888 