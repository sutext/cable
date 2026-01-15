
FROM golang:latest AS builder
WORKDIR /app
# copy all files to working directory
COPY . .
# go to cmd/cable directory
WORKDIR /app/cmd/cable
# download dependencies
RUN go mod download
# build program
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o ../../main

FROM alpine:latest
COPY --from=builder /app/main /
CMD ["/main"]
EXPOSE 1881 1882 1883 1884 1885/udp 1886/udp 6060 8888 