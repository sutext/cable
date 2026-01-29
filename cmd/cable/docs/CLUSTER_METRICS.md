# Cluster Metrics

Cable uses OpenTelemetry for distributed tracing and metrics collection. This document describes the available metrics, traces, and configuration options.

## Metrics

### cable.queue.length

- **Type:** Int64Gauge
- **Unit:** 1
- **Description:** Number of messages in the queue

### cable.online.users

- **Type:** Int64UpDownCounter
- **Unit:** 1
- **Description:** Number of online users

Labels:
- (none)

### cable.connect.duration

- **Type:** Float64Histogram
- **Unit:** ms
- **Description:** Connect packet processing duration in milliseconds

Labels:
- `ack_code`: ACK code returned during connection

### cable.message.duration

- **Type:** Float64Histogram
- **Unit:** ms
- **Description:** Message packet processing duration in milliseconds

Labels:
- `kind`: Message kind (integer)
- `isok`: Whether the operation succeeded (true/false)
- `inout`: Direction of the message (in/out)
- `network`: Network type

### cable.request.duration

- **Type:** Float64Histogram
- **Unit:** ms
- **Description:** Request packet processing duration in milliseconds

Labels:
- `isok`: Whether the operation succeeded (true/false)
- `method`: Request method
- `network`: Network type
- `inout`: Direction of the request (in/out)

### cable.redis.duration

- **Type:** Float64Histogram
- **Unit:** ms
- **Description:** Redis processing duration in milliseconds

Labels:
- `isok`: Whether the operation succeeded (true/false)
- `cmd`: Redis command name

### cable.kafka.duration

- **Type:** Float64Histogram
- **Unit:** ms
- **Description:** Kafka processing duration in milliseconds

Labels:
- `op`: Kafka operation type
- `topic`: Kafka topic name
- `isok`: Whether the operation succeeded (true/false)

## Traces

### cable.connect.in

- **Kind:** Server
- **Description:** Connect packet processing trace
- **Attributes:**
  - (none additional)

### cable.message.{inout}

- **Kind:** Server (in) / Client (out)
- **Description:** Message packet processing trace
- **Attributes:**
  - `cable.message.kind`: Message kind (integer)
  - `cable.message.network`: Network type

### cable.request.{inout}

- **Kind:** Server (in) / Client (out)
- **Description:** Request packet processing trace
- **Attributes:**
  - `cable.request.method`: Request method
  - `cable.request.network`: Network type

### cable.redis.{cmd}

- **Kind:** Client
- **Description:** Redis command execution trace
- **Attributes:**
  - (none additional)

### cable.kafka.{op}

- **Kind:** Client
- **Description:** Kafka operation trace
- **Attributes:**
  - `cable.kafka.topic`: Kafka topic name

## Configuration

### Tracing Configuration

```yaml
trace:
  enabled: true
  otlp_endpoint: "localhost:4317"
```

- **enabled:** Enable/disable tracing
- **otlp_endpoint:** OTLP gRPC endpoint for trace export

### Metrics Configuration

```yaml
metrics:
  enabled: true
  otlp_endpoint: "http://localhost:4318/v1/metrics"
  tenant_id: "your-tenant-id"
  interval: 10
```

- **enabled:** Enable/disable metrics collection
- **otlp_endpoint:** OTLP HTTP endpoint for metric export
- **tenant_id:** Tenant ID for multi-tenancy (used as X-Scope-OrgID header)
- **interval:** Export interval in seconds (default: 10)

## Resource Attributes

All metrics and traces include the following resource attributes:
- `service.name`: Service name
- `service.namespace`: Service namespace
- `service.version`: Service version
- `service.instance.id`: Unique broker instance ID

## Example Prometheus Queries

### Online Users

```promql
sum(cable_online_users)
```

### Average Kafka Duration by Topic

```promql
sum(rate(cable_kafka_duration_sum{topic="your-topic"}[5m]))
  /
sum(rate(cable_kafka_duration_count{topic="your-topic"}[5m]))
```

### 95th Percentile Message Duration

```promql
histogram_quantile(0.95, sum(rate(cable_message_duration_bucket[5m])) by (le))
```

### Redis Error Rate

```promql
sum(rate(cable_redis_duration_bucket{isok="false"}[5m]))
  /
sum(rate(cable_redis_duration_count[5m]))
```

### Queue Length by Broker

```promql
cable_queue_length
```
