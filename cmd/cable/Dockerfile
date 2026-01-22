
FROM golang:latest AS builder
WORKDIR /app
# copy all files to working directory
COPY . .
# download dependencies
RUN go mod download -C cmd/cable 
# build program
RUN CGO_ENABLED=0 GOOS=linux go build -C cmd/cable -ldflags="-s -w" -o ../../main

FROM alpine:latest
COPY --from=builder /app/main /
CMD ["/main"]