# Cable

Cable is a high-performance, distributed real-time communication platform that supports multiple network protocols, providing reliable message transmission services for large-scale real-time applications.

## Core Features

### 🔗 Multi-protocol Support
- **TCP** - Traditional reliable transmission protocol
- **UDP** - Low-latency data transmission
- **WebSocket** - Browser-compatible full-duplex communication
- **QUIC** - Next-generation transport protocol based on UDP, supporting 0-RTT connections

### 📡 Distributed Architecture
- **Raft Consensus Algorithm** - Ensures cluster data consistency
- **Automatic Discovery Mechanism** - Nodes automatically join the cluster
- **Multi-node Deployment** - Supports horizontal scaling
- **Automatic Failover** - High availability guarantee

### 📱 Real-time Message Transmission
- **One-to-one Messages** - Point-to-point real-time communication
- **Group Messages** - Channel-based broadcasting mechanism
- **Message Routing** - Intelligent routing to target clients
- **Message Queue** - Ensures reliable message delivery

### 🛠️ Integration Ecosystem
- **Redis** - Used for user channel management
- **Kafka** - Used for message persistence and cross-service communication
- **gRPC** - Used for inter-node communication

## Architecture Design

### Core Components

```
┌───────────────────────────────────────────────────────────────────────┐
│                            Cable Cluster                              │
├─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┤
│ Broker 1│ Broker 2│ Broker 3│ Broker 4│ Broker 5│ Broker 6│  ...    │
└─────────┴─────────┴─────────┴─────────┴─────────┴─────────┴─────────┘
          │         │         │         │         │         │
          └─────────┴─────────┴─────────┴─────────┴─────────┴─────────┐
                                                                       │
┌───────────────────────────────────────────────────────────────────────┘
│                              External Integration                      │
├───────────┬─────────────┬────────────────────────────────────────────┤
│   Redis   │    Kafka    │               Client Applications             │
└───────────┴─────────────┴────────────────────────────────────────────┘
```

### Component Description

- **Broker** - Core node responsible for client connection management and message routing
- **Raft** - Distributed consensus protocol ensuring cluster state consistency
- **Redis** - Stores user channel information
- **Kafka** - For message persistence and cross-service communication
- **Client SDK** - Provides client libraries in multiple languages

## Quick Start

### Prerequisites

- Go 1.25.2 or higher
- Redis 6.0 or higher
- Kafka 2.8.0 or higher

### Build

```bash
# Build the main program
./build.sh

# Build the test program
./build_tester.sh
```

### Run

```bash
# Enter the command directory
cd cmd/cable

# Run in single-node mode
go run cable.go

# Or run with a configuration file
go run cable.go -config cable.config.yaml
```

### Cluster Mode

1. Configure multiple node configuration files, modifying `brokerID` and ports
2. Start each node sequentially
3. Nodes will automatically discover and form a cluster

## Configuration

### Configuration File Format

```yaml
# Basic configuration
brokerID: 1
logLevel: "info"
pprof: false

# Cluster configuration
initSize: 3
peers: []

# Listener configuration
listeners:
  - network: "tcp"
    port: 8000
    autoStart: true
    queueSize: 1024
    tls:
      certFile: ""
      keyFile: ""

# Redis configuration
redis:
  address: "localhost:6379"
  password: ""
  db: 0

# Kafka configuration
kafka:
  brokers: ["localhost:9092"]
  userUpTopic: "cable-user-up"
  userDownTopic: "cable-user-down"
  groupUpTopic: "cable-group-up"
  groupDownTopic: "cable-group-down"
```

### Configuration Items

| Configuration Item | Type | Description |
|-------------------|------|-------------|
| brokerID | uint64 | Unique node identifier |
| logLevel | string | Log level (debug, info, warn, error) |
| initSize | int32 | Initial cluster size |
| listeners | []Listener | Network listener configuration |
| redis.address | string | Redis server address |
| kafka.brokers | []string | Kafka broker list |

## Development Guide

### Directory Structure

```
├── backoff/          # Backoff algorithm implementation
├── client/           # Client SDK
├── cluster/          # Cluster core implementation
├── cmd/              # Command-line tools
│   ├── cable/        # Main program
│   └── tester/       # Test program
├── coder/            # Encoding/decoding module
├── internal/         # Internal utility libraries
├── packet/           # Packet definitions
└── server/           # Server core
```

### Core API

#### Server-side

```go
// Create a broker instance
broker := cluster.NewBroker(
    cluster.WithBrokerID(1),
    cluster.WithHandler(handler),
    cluster.WithInitSize(3),
    cluster.WithListeners(listeners),
)

// Start the service
if err := broker.Start(); err != nil {
    log.Fatal(err)
}

// Send a message to a user
total, success, err := broker.SendToUser(ctx, userID, message)

// Send a message to a channel
total, success, err := broker.SendToChannel(ctx, channel, message)
```

#### Client-side

```go
// Create a client instance
client := client.NewClient(
    client.WithNetwork("tcp"),
    client.WithAddress("localhost:8000"),
)

// Connect to the server
if err := client.Connect(); err != nil {
    log.Fatal(err)
}

// Send a message
if err := client.SendMessage(message); err != nil {
    log.Fatal(err)
}
```

## Testing

### Unit Tests

```bash
# Run all unit tests
go test ./...

# Run tests for a specific package
go test ./internal/safe
```

### Integration Tests

Use the `tester` tool for integration testing:

```bash
cd cmd/tester
go run main.go
```

## Deployment

### Docker Deployment

```bash
# Build the Docker image
docker build -t cable -f cmd/cable/Dockerfile .

# Run the container
docker run -d --name cable -p 8000:8000 cable
```

### Kubernetes Deployment

```bash
# Deploy the cluster
kubectl apply -f cmd/cable/k8s/cable-cluster.yaml

# Check status
kubectl get pods
```

## Performance Metrics

- **Connections** - Single node supports 100,000+ concurrent connections
- **Message Latency** - Average latency < 10ms
- **Throughput** - 100,000+ messages/second
- **Failover** - Automatic recovery from node failure, recovery time < 10 seconds

## Contributing

Contributions are welcome! Please follow these steps:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

## License

This project is licensed under the Apache License - see the [LICENSE](LICENSE) file for details

## Contact

- **Project Address** - https://github.com/sutext/cable
- **Issue Tracking** - https://github.com/sutext/cable/issues
- **Discussion Community** - https://github.com/sutext/cable/discussions

## Roadmap

- [x] Multi-protocol support (TCP, UDP, WebSocket, QUIC)
- [x] Distributed Raft cluster
- [x] Redis integration
- [x] Kafka integration
- [ ] Client-side load balancing
- [ ] Message persistence
- [ ] Monitoring dashboard
- [ ] More comprehensive management API

---

**Cable** - Building reliable real-time communication systems
