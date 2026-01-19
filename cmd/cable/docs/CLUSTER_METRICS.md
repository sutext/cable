# 集群级消息吞吐量统计指南

## 架构概述

Cable 集群使用 Prometheus 进行分布式指标收集和聚合：

1. **每个 broker 节点**独立暴露 metrics 端口（默认 8888)
2. **Prometheus** 定期抓取所有节点的指标数据
3. **PromQL 查询**聚合计算集群总吞吐量

## 指标说明

### 上行消息指标

- `cable_messages_up_total{broker_id, kind}`: 上行消息总数
- `cable_messages_up_duration_seconds{broker_id, kind}`: 上行消息处理时长

### 下行消息指标

- `cable_messages_down_total{broker_id, kind, target_type}`: 下行消息总数
- `cable_messages_down_duration_seconds{broker_id, kind, target_type}`: 下行消息处理时长

**标签说明**:
- `broker_id`: broker 节点 ID
- `kind`: 消息类型（数字）
- `target_type`: 目标类型（all/user/channel）

## PromQL 查询示例

### 1. 集群总上行消息速率

```promql
# 每秒上行消息数（所有 broker，所有消息类型）
sum(rate(cable_messages_up_total[5m]))

# 每秒上行消息数（按消息类型分组）
sum(rate(cable_messages_up_total[5m])) by (kind)

# 每秒上行消息数（按 broker 分组）
sum(rate(cable_messages_up_total[5m])) by (broker_id)

# 每秒上行消息数（按 broker 和消息类型分组）
sum(rate(cable_messages_up_total[5m])) by (broker_id, kind)
```

### 2. 集群总下行消息速率

```promql
# 每秒下行消息数（所有 broker，所有目标类型）
sum(rate(cable_messages_down_total[5m]))

# 每秒下行消息数（按目标类型分组）
sum(rate(cable_messages_down_total[5m])) by (target_type)

# 每秒下行消息数（按 broker 和目标类型分组）
sum(rate(cable_messages_down_total[5m])) by (broker_id, target_type)

# 每秒下行消息数（按消息类型和目标类型分组）
sum(rate(cable_messages_down_total[5m])) by (kind, target_type)
```

### 3. 集群总吞吐量（上行 + 下行）

```promql
# 每秒总消息数（上行 + 下行）
sum(rate(cable_messages_up_total[5m])) + sum(rate(cable_messages_down_total[5m]))

# 按消息类型分组
sum(rate(cable_messages_up_total[5m])) by (kind) + sum(rate(cable_messages_down_total[5m])) by (kind)
```

### 4. 消息处理延迟

```promql
# 上行消息平均处理延迟（秒）
avg(rate(cable_messages_up_duration_seconds_sum[5m]) / rate(cable_messages_up_duration_seconds_count[5m]))

# 下行消息平均处理延迟（秒）
avg(rate(cable_messages_down_duration_seconds_sum[5m]) / rate(cable_messages_down_duration_seconds_count[5m]))

# 按 broker 分组的上行消息延迟
avg(rate(cable_messages_up_duration_seconds_sum[5m]) / rate(cable_messages_up_duration_seconds_count[5m])) by (broker_id)

# 按 target_type 分组的下行消息延迟
avg(rate(cable_messages_down_duration_seconds_sum[5m]) / rate(cable_messages_down_duration_seconds_count[5m])) by (target_type)
```

### 5. P99 延迟

```promql
# 上行消息 P99 延迟
histogram_quantile(0.99, sum(rate(cable_messages_up_duration_seconds_bucket[5m])) by (le, broker_id))

# 下行消息 P99 延迟
histogram_quantile(0.99, sum(rate(cable_messages_down_duration_seconds_bucket[5m])) by (le, broker_id, target_type))
```

### 6. Broker 负载均衡分析

```promql
# 各 broker 的上行消息占比
sum(rate(cable_messages_up_total[5m])) by (broker_id) / sum(rate(cable_messages_up_total[5m]))

# 各 broker 的下行消息占比
sum(rate(cable_messages_down_total[5m])) by (broker_id) / sum(rate(cable_messages_down_total[5m]))
```

## Grafana 面板示例

### 面板 1: 集群总吞吐量

```promql
sum(rate(cable_messages_up_total[5m])) + sum(rate(cable_messages_down_total[5m]))
```

### 面板 2: 各 Broker 吞吐量对比

```promql
sum(rate(cable_messages_up_total[5m])) by (broker_id) + sum(rate(cable_messages_down_total[5m])) by (broker_id)
```

### 面板 3: 消息类型分布

```promql
sum(rate(cable_messages_up_total[5m])) by (kind)
```

### 面板 4: 消息处理延迟趋势

```promql
avg(rate(cable_messages_up_duration_seconds_sum[5m]) / rate(cable_messages_up_duration_seconds_count[5m]))
```

## 配置说明

### Broker 配置示例

每个 broker 需要配置不同的 metrics 端口：

```yaml
# broker1.yaml
brokerid: 1
metrics:
  enabled: true
  port: 9090
  path: /metrics

# broker2.yaml
brokerid: 2
metrics:
  enabled: true
  port: 9091
  path: /metrics

# broker3.yaml
brokerid: 3
metrics:
  enabled: true
  port: 9092
  path: /metrics
```

### Prometheus 配置

参考 [prometheus.yml](prometheus.yml) 配置文件。

## 监控告警规则示例

```yaml
groups:
  - name: cable_cluster_alerts
    rules:
      # 集群总吞吐量过低
      - alert: LowClusterThroughput
        expr: sum(rate(cable_messages_up_total[5m])) + sum(rate(cable_messages_down_total[5m])) < 100
        for: 5m
        annotations:
          summary: "集群吞吐量过低"

      # 单个 broker 吞吐量异常
      - alert: BrokerThroughputAnomaly
        expr: |
          abs(
            sum(rate(cable_messages_up_total[5m])) by (broker_id) 
            - avg(sum(rate(cable_messages_up_total[5m])))
          ) / avg(sum(rate(cable_messages_up_total[5m])) > 0.5
        for: 10m
        annotations:
          summary: "Broker {{ $labels.broker_id }} 吞吐量异常"

      # 消息处理延迟过高
      - alert: HighMessageLatency
        expr: |
          avg(rate(cable_messages_up_duration_seconds_sum[5m]) / rate(cable_messages_up_duration_seconds_count[5m])) > 1
        for: 5m
        annotations:
          summary: "消息处理延迟过高"
```
