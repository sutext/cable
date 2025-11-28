# 构建阶段
FROM golang:latest AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o main cmd/cable/*.go

# 运行时阶段：使用 Distroless 静态镜像
FROM alpine:latest
COPY --from=builder /app/main /
CMD ["/main"]
EXPOSE 1883 1884/udp 4567 6060 8888 