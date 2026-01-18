package main

import (
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
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
type redisConfig struct {
	DB         int    `yaml:"db"`
	Address    string `yaml:"address"`
	Password   string `yaml:"password"`
	UserPrefix string `yaml:"userPrefix"`
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
	Enabled      bool    `yaml:"enabled"`
	OTLPEndpoint string  `yaml:"otlpEndpoint"`
	ServiceName  string  `yaml:"serviceName"`
	SamplerRatio float64 `yaml:"samplerRatio"`
}

type metricsConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    uint16 `yaml:"port"`
	Path    string `yaml:"path"`
}

type config struct {
	Pprof        bool                         `yaml:"pprof"`
	BrokerID     uint64                       `yaml:"brokerid"`
	InitSize     int32                        `yaml:"initSize"`
	GrpcPort     uint16                       `yaml:"grpcPort"`
	HTTPPort     uint16                       `yaml:"httpPort"`
	PeerPort     uint16                       `yaml:"peerPort"`
	LogLevel     string                       `yaml:"logLevel"`
	Redis        redisConfig                  `yaml:"redis"`
	Trace        traceConfig                  `yaml:"trace"`
	Metrics      metricsConfig                `yaml:"metrics"`
	Listeners    []listener                   `yaml:"listeners"`
	KafkaBrokers []string                     `yaml:"kafkaBrokers"`
	MessageRoute map[packet.MessageKind]route `yaml:"messageRoute"`
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

	return cfg, nil
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
