# cable

High-performance network transmission protocol, used for building real-time push, chat or game applications.

## Features

### Core Features
- **Multi-protocol Support**: TCP, UDP, WebSocket, QUIC, gRPC
- **Real-time Messaging**: One-to-one, one-to-many, and broadcast messages
- **Channel Management**: Join/leave channels, channel-based messaging
- **Cluster Support**: Auto-discovery, distributed message routing
- **High Availability**: Fault tolerance, connection resilience
- **QoS Support**: Quality of Service for reliable message delivery
- **Keepalive Mechanism**: Automatic connection monitoring
- **Request-Response Model**: For synchronous communication

### Advanced Features
- **Connection Management**: Kick connections or users, online status query
- **Extensible Architecture**: Easy to add new transports or features
- **Built-in Retry Logic**: Automatic reconnection with backoff
- **Docker & Kubernetes Support**: Easy deployment and scaling
- **HTTP API**: For external system integration

## Architecture

### Components
- **Broker**: Core message routing and cluster management
- **Client**: Client library for connecting to brokers
- **Server**: Multi-protocol server framework
- **Packet**: Message serialization and deserialization
- **Internal**: Utility packages for keepalive, queues, and safe data structures

### Message Flow
1. Client connects to Broker with authentication
2. Broker registers client and manages connection
3. Messages are routed based on destination (user/channel/all)
4. For cluster deployment, messages are synchronized across nodes
5. Keepalive ensures connection health

## Getting Started

### Prerequisites
- Go 1.25.2 or higher
- Git

### Installation
```bash
go get -u sutext.github.io/cable
```

### Building
```bash
# Build the cable server
go build -o cable ./cmd/cable

# Build the tester for testing
go build -o tester ./cmd/tester
```

### Quick Run

#### Start a Single Broker
```bash
./cable --listen tcp://0.0.0.0:8000 --listen ws://0.0.0.0:8001
```

#### Cluster Deployment
```bash
# Node 1
./cable --id broker-1 --listen tcp://0.0.0.0:8000 --peer-port 9000

# Node 2
./cable --id broker-2 --listen tcp://0.0.0.0:8002 --peer-port 9002 --peers broker-1@localhost:9000

# Node 3
./cable --id broker-3 --listen tcp://0.0.0.0:8004 --peer-port 9004 --peers broker-1@localhost:9000,broker-2@localhost:9002
```

## Usage

### Server Side

#### Basic Setup
```go
package main

import (
    "context"
    "fmt"
    "sutext.github.io/cable/broker"
    "sutext.github.io/cable/packet"
)

func main() {
    // Create a new broker
    b := broker.NewBroker(
        broker.WithBrokerID("broker-1"),
        broker.WithListeners(map[broker.Transport]string{
            broker.TCP:  "0.0.0.0:8000",
            broker.WS:   "0.0.0.0:8001",
        }),
        broker.WithHandler(&MyHandler{}),
    )

    // Start the broker
    if err := b.Start(); err != nil {
        panic(err)
    }
    defer b.Shutdown(context.Background())

    fmt.Println("Broker started successfully")
    select {}
}

type MyHandler struct{}

func (h *MyHandler) OnConnect(p *packet.Connect) packet.ConnectCode {
    // Authenticate client
    return packet.ConnectAccepted
}

func (h *MyHandler) OnMessage(m *packet.Message, id *packet.Identity) error {
    // Handle incoming message
    return nil
}

func (h *MyHandler) OnClosed(id *packet.Identity) {
    // Handle connection closed
}

func (h *MyHandler) GetChannels(uid string) ([]string, error) {
    // Return channels for user
    return []string{"default"}, nil
}
```

### Client Side

#### Basic Connection
```go
package main

import (
    "fmt"
    "time"
    "sutext.github.io/cable/client"
    "sutext.github.io/cable/packet"
    "sutext.github.io/cable/xlog"
)

func main() {
    // Create a new client
    c := client.New("tcp://localhost:8000",
        client.WithLogger(xlog.New("client", xlog.DebugLevel)),
        client.WithHandler(&MyClientHandler{}),
    )

    // Connect to broker
    identity := &packet.Identity{
        UserID:   "user-1",
        ClientID: "client-1",
    }
    c.Connect(identity)

    // Wait for connection to be ready
    for !c.IsReady() {
        time.Sleep(time.Millisecond * 100)
    }

    // Send a message
    msg := packet.NewMessage()
    msg.Topic = "chat"
    msg.Data = []byte("Hello, cable!")
    c.SendMessage(context.Background(), msg)

    // Send a request
    req := packet.NewRequest()
    req.Method = "ping"
    resp, err := c.SendRequest(context.Background(), req)
    if err == nil {
        fmt.Println("Response:", string(resp.Data))
    }
}

type MyClientHandler struct{}

func (h *MyClientHandler) OnMessage(msg *packet.Message) error {
    fmt.Println("Received message:", string(msg.Data))
    return nil
}

func (h *MyClientHandler) OnStatus(status client.Status) {
    fmt.Println("Status changed:", status)
}

func (h *MyClientHandler) OnRequest(req *packet.Request) (*packet.Response, error) {
    // Handle server request
    return packet.NewResponse(req.ID, packet.ResponseSuccess, []byte("pong")), nil
}
```

## API Reference

### Broker API

#### Message Methods
- `SendToAll(ctx context.Context, m *packet.Message) (total, success int32, err error)`
- `SendToUser(ctx context.Context, uid string, m *packet.Message) (total, success int32, err error)`
- `SendToChannel(ctx context.Context, channel string, m *packet.Message) (total, success int32, err error)`

#### Channel Methods
- `JoinChannel(ctx context.Context, uid string, channels ...string) (count int32, err error)`
- `LeaveChannel(ctx context.Context, uid string, channels ...string) (count int32, err error)`

#### Connection Methods
- `IsOnline(ctx context.Context, uid string) (ok bool)`
- `KickConn(ctx context.Context, cid string)`
- `KickUser(ctx context.Context, uid string)`

### Client API

#### Connection Methods
- `Connect(identity *packet.Identity)`
- `IsReady() bool`
- `Status() Status`

#### Message Methods
- `SendMessage(ctx context.Context, p *packet.Message) error`
- `SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)`

## Configuration

### Broker Configuration
| Option | Description | Default |
|--------|-------------|---------|
| --id | Broker ID | Auto-generated |
| --listen | Listen addresses (e.g., tcp://0.0.0.0:8000) | tcp://0.0.0.0:8000 |
| --peer-port | Peer communication port | 9000 |
| --peers | Initial peer list (id@ip:port) | None |
| --http-port | HTTP API port | 8080 |

### Client Configuration
| Option | Description | Default |
|--------|-------------|---------|
| Network | Connection transport | TCP |
| Address | Broker address | None |
| Logger | Log level and format | Info |
| RetryInterval | Initial retry interval | 1s |
| MaxRetries | Maximum retry attempts | 5 |
| PingInterval | Keepalive ping interval | 30s |
| PingTimeout | Ping response timeout | 10s |

## Deployment

### Docker
```bash
docker build -t cable ./cmd/cable
docker run -p 8000:8000 -p 9000:9000 cable --listen tcp://0.0.0.0:8000 --peer-port 9000
```

### Kubernetes
```bash
kubectl apply -f ./docker/cable-cluster.yaml
```

## Testing

### Run Tests
```bash
go test ./...
```

### Using the Tester
```bash
./tester --server tcp://localhost:8000 --clients 100 --messages 1000
```

## Use Cases

### Chat Applications
- Real-time messaging between users
- Group chats using channels
- Presence indicators

### Gaming
- In-game chat
- Game state synchronization
- Player notifications

### IoT
- Device telemetry
- Command and control
- Alert notifications

### Financial Applications
- Real-time market data
- Trade notifications
- Order updates

## Contributing

We welcome contributions! Please follow these steps:

1. Fork the repository
2. Create a new branch for your feature/bugfix
3. Make your changes
4. Run tests to ensure everything works
5. Submit a pull request

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Thanks to all contributors who have helped shape this project
- Inspired by various real-time messaging protocols and frameworks

## Contact

For questions, issues, or suggestions, please:
- Open an issue on GitHub
- Submit a pull request
- Join our community

---

Built with ❤️ for real-time applications
