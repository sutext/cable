package main

import (
	"log/slog"
	"os"

	"gopkg.in/yaml.v3"
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
	DB       int    `yaml:"db"`
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
}
type kafkaConfig struct {
	Brokers        []string `yaml:"brokers"`
	UserUpTopic    string   `yaml:"userUpTopic"`
	GroupUpTopic   string   `yaml:"groupUpTopic"`
	UserDownTopic  string   `yaml:"userDownTopic"`
	GroupDownTopic string   `yaml:"groupDownTopic"`
}
type config struct {
	Redis     redisConfig `yaml:"redis"`
	Kafka     kafkaConfig `yaml:"kafka"`
	Pprof     bool        `yaml:"pprof"`
	BrokerID  uint64      `yaml:"brokerid"`
	InitSize  int32       `yaml:"initSize"`
	HTTPPort  uint16      `yaml:"httpPort"`
	PeerPort  uint16      `yaml:"peerPort"`
	LogLevel  string      `yaml:"logLevel"`
	Listeners []listener  `yaml:"listeners"`
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
