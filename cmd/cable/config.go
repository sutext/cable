package main

import (
	"fmt"
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
)

type tlsConfig struct {
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}
type listener struct {
	Port      uint16     `yaml:"port"`
	Network   string     `yaml:"network"`
	QueueSize int32      `yaml:"queueSize"`
	AutoStart bool       `yaml:"autoStart"`
	TLS       *tlsConfig `yaml:"tls"`
}
type kafkaConfig struct {
	Enabled bool     `yaml:"enabled"`
	Brokers []string `yaml:"brokers"`
}
type redisSingle struct {
	DB         int    `yaml:"db"`
	Enabled    bool   `yaml:"enabled"`
	Address    string `yaml:"address"`
	Password   string `yaml:"password"`
	UserPrefix string `yaml:"userPrefix"`
}
type redisCluster struct {
	Enabled    bool     `yaml:"enabled"`
	Password   string   `yaml:"password"`
	Addresses  []string `yaml:"addresses"`
	UserPrefix string   `yaml:"userPrefix"`
}
type resendType string

const (
	resendNone      resendType = "none"
	resendToAll     resendType = "all"
	resendToUser    resendType = "user"
	resendToChannel resendType = "channel"
)

type route struct {
	Enabled    bool       `yaml:"enabled"`
	ResendType resendType `yaml:"resendType"`
	KafkaTopic string     `yaml:"kafkaTopic"`
}
type traceConfig struct {
	Enabled      bool   `yaml:"enabled"`
	OTLPEndpoint string `yaml:"otlpEndpoint"`
}

type metricsConfig struct {
	Enabled      bool   `yaml:"enabled"`
	TenantID     string `yaml:"tenantID"`
	Interval     int    `yaml:"interval"`
	OTLPEndpoint string `yaml:"otlpEndpoint"`
}

type config struct {
	Pprof          bool                         `yaml:"pprof"`
	BrokerID       uint64                       `yaml:"brokerid"`
	GrpcPort       uint16                       `yaml:"grpcPort"`
	HTTPPort       uint16                       `yaml:"httpPort"`
	PeerPort       uint16                       `yaml:"peerPort"`
	LogLevel       string                       `yaml:"logLevel"`
	LogFormat      string                       `yaml:"logFormat"`
	ClusterSize    int32                        `yaml:"clusterSize"`
	ServiceName    string                       `yaml:"serviceName"`
	ServiceVersion string                       `yaml:"serviceVersion"`
	Trace          traceConfig                  `yaml:"trace"`
	Metrics        metricsConfig                `yaml:"metrics"`
	Kafka          kafkaConfig                  `yaml:"kafka"`
	Redis          redisSingle                  `yaml:"redis"`
	RedisCluster   redisCluster                 `yaml:"redisCluster"`
	MessageRoute   map[packet.MessageKind]route `yaml:"messageRoute"`
	Listeners      []listener                   `yaml:"listeners"`
}

func (c *config) String() string {
	data, _ := yaml.Marshal(c)
	return string(data)
}
func readConfig(path string) (*config, error) {
	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse yaml to config struct
	cfg := &config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Validate config
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.BrokerID == 0 {
		cfg.BrokerID = cluster.BrokerID()
	}
	return cfg, nil
}
func (c *config) validate() error {
	if c.GrpcPort == 0 {
		return fmt.Errorf("grpc port is 0")
	}
	if c.HTTPPort == 0 {
		return fmt.Errorf("http port is 0")
	}
	if c.PeerPort == 0 {
		return fmt.Errorf("peer port is 0")
	}
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return fmt.Errorf("invalid log format: %s", c.LogFormat)
	}
	if c.ClusterSize < 0 {
		return fmt.Errorf("invalid cluster size: %d", c.ClusterSize)
	}
	if c.ServiceName == "" {
		return fmt.Errorf("service name is empty")
	}
	if c.ServiceVersion == "" {
		return fmt.Errorf("service version is empty")
	}
	if c.Trace.Enabled && c.Trace.OTLPEndpoint == "" {
		return fmt.Errorf("trace enabled but service name is empty")
	}
	if c.Metrics.Enabled {
		if c.Metrics.Interval < 1 {
			return fmt.Errorf("metrics interval is less than 1")
		}
		if c.Metrics.OTLPEndpoint == "" {
			return fmt.Errorf("metrics enabled but OTLP endpoint is empty")
		}
		if c.Metrics.TenantID == "" {
			return fmt.Errorf("metrics enabled but tenant ID is empty")
		}
	}
	if c.Kafka.Enabled {
		if len(c.Kafka.Brokers) == 0 {
			return fmt.Errorf("kafka enabled but brokers are empty")
		}
		if c.Kafka.Brokers[0] == "" {
			return fmt.Errorf("kafka enabled but first broker is empty")
		}
	}
	if c.RedisCluster.Enabled {
		if len(c.RedisCluster.Addresses) == 0 {
			return fmt.Errorf("redis cluster enabled but addresses are empty")
		}
		if c.RedisCluster.Addresses[0] == "" {
			return fmt.Errorf("redis cluster enabled but first address is empty")
		}
		if c.RedisCluster.UserPrefix == "" {
			return fmt.Errorf("redis cluster enabled but user prefix is empty")
		}
	}
	if c.Redis.Enabled {
		if c.Redis.Address == "" {
			return fmt.Errorf("redis enabled but address is empty")
		}
		if c.Redis.UserPrefix == "" {
			return fmt.Errorf("redis enabled but user prefix is empty")
		}
	}
	if c.MessageRoute == nil {
		return fmt.Errorf("message route is nil")
	}
	for k, v := range c.MessageRoute {
		if k > 63 {
			return fmt.Errorf("invalid message kind: %d", k)
		}
		if v.ResendType != resendNone && v.ResendType != resendToAll && v.ResendType != resendToUser && v.ResendType != resendToChannel {
			return fmt.Errorf("invalid resend type: %s", v.ResendType)
		}
	}
	if len(c.Listeners) == 0 {
		return fmt.Errorf("no listeners defined")
	}
	for _, l := range c.Listeners {
		if l.Port == 0 {
			return fmt.Errorf("listener port is 0")
		}
		if l.Network != "tcp" && l.Network != "tls" && l.Network != "ws" && l.Network != "wss" && l.Network != "udp" && l.Network != "quic" {
			return fmt.Errorf("invalid listener network: %s", l.Network)
		}
		if l.QueueSize < 0 {
			return fmt.Errorf("invalid listener queue size: %d", l.QueueSize)
		}
		if l.TLS != nil {
			if l.TLS.CertFile == "" {
				return fmt.Errorf("listener tls cert file is empty")
			}
			if l.TLS.KeyFile == "" {
				return fmt.Errorf("listener tls key file is empty")
			}
		}
	}
	return nil
}
func (c *config) Level() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
