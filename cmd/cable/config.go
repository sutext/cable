package main

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/log/global"
	logsdk "go.opentelemetry.io/otel/sdk/log"
	resource "go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
	"sutext.github.io/cable/cluster"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
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
type redisConfig struct {
	DB         int      `yaml:"db"`
	Enabled    bool     `yaml:"enabled"`
	Password   string   `yaml:"password"`
	Addresses  []string `yaml:"addresses"`
	AuthPrefix string   `yaml:"authPrefix"`
	JoinPrefix string   `yaml:"joinPrefix"`
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
type loggerConfig struct {
	Level        string `yaml:"level"`
	Format       string `yaml:"format"`
	OTLPEndpoint string `yaml:"otlpEndpoint"`
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
type authConfig struct {
	Method  string `yaml:"method"`  // "jwt" or "static" or "redis" or "remote"
	Secret  string `yaml:"secret"`  // only used when method is "jwt" or "static"
	AuthURL string `yaml:"authURL"` // only used when method is "remote"
}

type config struct {
	Auth           authConfig                   `yaml:"auth"`
	Pprof          bool                         `yaml:"pprof"`
	BrokerID       uint64                       `yaml:"brokerid"`
	GrpcPort       uint16                       `yaml:"grpcPort"`
	HTTPPort       uint16                       `yaml:"httpPort"`
	PeerPort       uint16                       `yaml:"peerPort"`
	ClusterSize    int32                        `yaml:"clusterSize"`
	ServiceName    string                       `yaml:"serviceName"`
	ServiceVersion string                       `yaml:"serviceVersion"`
	Trace          traceConfig                  `yaml:"trace"`
	Logger         loggerConfig                 `yaml:"logger"`
	Metrics        metricsConfig                `yaml:"metrics"`
	Kafka          kafkaConfig                  `yaml:"kafka"`
	Redis          redisConfig                  `yaml:"redis"`
	MessageRoute   map[packet.MessageKind]route `yaml:"messageRoute"`
	Listeners      []listener                   `yaml:"listeners"`
	_logger        *xlog.Logger
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
	if c.Logger.Level != "info" && c.Logger.Level != "debug" && c.Logger.Level != "warn" && c.Logger.Level != "error" && c.Logger.Level != "fatal" {
		return fmt.Errorf("logger enabled but level is invalid")
	}
	if c.Logger.Format != "json" && c.Logger.Format != "console" {
		return fmt.Errorf("logger enabled but format is invalid")
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
	if c.Redis.Enabled {
		if len(c.Redis.Addresses) == 0 {
			return fmt.Errorf("redis enabled but address is empty")
		}
		if c.Redis.JoinPrefix == "" {
			return fmt.Errorf("redis enabled but user prefix is empty")
		}
	}
	if c.Auth.Method != "pass" && c.Auth.Method != "jwt" && c.Auth.Method != "static" && c.Auth.Method != "redis" && c.Auth.Method != "remote" {
		return fmt.Errorf("invalid auth method: %s", c.Auth.Method)
	}
	if c.Auth.Method == "remote" && c.Auth.AuthURL == "" {
		return fmt.Errorf("auth method is remote but auth URL is empty")
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
func (c *config) GetLogger() *xlog.Logger {
	if c._logger != nil {
		return c._logger
	}
	if c.Logger.OTLPEndpoint == "" {
		c._logger = c.consoleLogger()
		return c._logger
	}
	l, err := c.otlpLogger()
	if err != nil {
		c._logger = c.consoleLogger()
	}
	c._logger = xlog.New(l)
	return c._logger
}
func (c *config) consoleLogger() *xlog.Logger {
	if c.Logger.Format == "json" {
		return xlog.NewJSON(xlog.ParseLevel(c.Logger.Level), zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return &ctxCore{core: c}
		}))
	} else {
		return xlog.NewText(xlog.ParseLevel(c.Logger.Level), zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return &ctxCore{core: c}
		}))
	}
}
func (c *config) otlpLogger() (*zap.Logger, error) {
	exporter, err := otlploggrpc.New(context.Background(),
		otlploggrpc.WithEndpoint(c.Logger.OTLPEndpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	loggerProvider := logsdk.NewLoggerProvider(
		logsdk.WithProcessor(logsdk.NewBatchProcessor(exporter)),
		logsdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(c.ServiceName),
		)),
	)
	global.SetLoggerProvider(loggerProvider)
	core := otelzap.NewCore(c.ServiceName)
	logger := zap.New(&lvlCore{Core: core, level: xlog.ParseLevel(c.Logger.Level).Level()})
	return logger, nil
}

type lvlCore struct {
	zapcore.Core
	level zapcore.Level
}

func (f *lvlCore) Enabled(level zapcore.Level) bool {
	return level >= f.level
}
func (f *lvlCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if entry.Level >= f.level {
		return ce.AddCore(entry, f)
	}
	return ce
}

type ctxCore struct {
	core zapcore.Core
}

func (f *ctxCore) Write(e zapcore.Entry, fields []zapcore.Field) error {
	if len(fields) == 0 {
		return f.core.Write(e, fields)
	}
	ctx, ok := fields[0].Interface.(context.Context)
	if !ok {
		return f.core.Write(e, fields)
	}
	fields = fields[1:]
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		sc := span.SpanContext()
		fields = append(fields, xlog.Str("trace_id", sc.TraceID().String()))
		fields = append(fields, xlog.Str("span_id", sc.SpanID().String()))
	}
	return f.core.Write(e, fields)
}
func (f *ctxCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if f.Enabled(entry.Level) {
		ce = ce.AddCore(entry, f)
	}
	return ce
}
func (f *ctxCore) With(fields []zapcore.Field) zapcore.Core {
	return f.core.With(fields)
}
func (f *ctxCore) Enabled(level zapcore.Level) bool {
	return f.core.Enabled(level)
}
func (f *ctxCore) Sync() error {
	return f.core.Sync()
}
