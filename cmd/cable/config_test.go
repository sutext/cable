package main

import (
	"os"
	"testing"

	"sutext.github.io/cable/packet"

	"gopkg.in/yaml.v3"
)

func TestReadConfig(t *testing.T) {
	// Create a temporary test config file
	testConfig := `
pprof: true
brokerid: 123
initSize: 10
httpPort: 8080
peerPort: 9090
logLevel: "debug"
redis:
  db: 1
  address: "localhost:6379"
  password: "testpass"
kafkaBrokers:
  - "kafka1:9092"
  - "kafka2:9092"
messageRoute:
  2:
    resendType: "user"
    kafkaTopic: "test-user-topic"
  3:
    resendType: "channel"
    kafkaTopic: "test-group-topic"
listeners:
  - network: "ws"
    port: 8081
    queueSize: 512
    autoStart: true
  - network: "wss"
    port: 8082
    queueSize: 1024
    autoStart: false
    tls:
      certFile: "test.crt"
      keyFile: "test.key"
`

	// Write test config to temporary file
	tempFile, err := os.CreateTemp("", "test-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.WriteString(testConfig); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Read and parse the config
	cfg, err := readConfig(tempFile.Name())
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	// Test Redis config
	if cfg.Redis.DB != 1 {
		t.Errorf("Expected Redis DB to be 1, got %d", cfg.Redis.DB)
	}
	if cfg.Redis.Address != "localhost:6379" {
		t.Errorf("Expected Redis Address to be 'localhost:6379', got '%s'", cfg.Redis.Address)
	}
	if cfg.Redis.Password != "testpass" {
		t.Errorf("Expected Redis Password to be 'testpass', got '%s'", cfg.Redis.Password)
	}

	// Test other basic configs
	if !cfg.Pprof {
		t.Errorf("Expected Pprof to be true, got false")
	}
	if cfg.BrokerID != 123 {
		t.Errorf("Expected BrokerID to be 123, got %d", cfg.BrokerID)
	}
	if cfg.InitSize != 10 {
		t.Errorf("Expected InitSize to be 10, got %d", cfg.InitSize)
	}
	if cfg.HTTPPort != 8080 {
		t.Errorf("Expected HTTPPort to be 8080, got %d", cfg.HTTPPort)
	}
	if cfg.PeerPort != 9090 {
		t.Errorf("Expected PeerPort to be 9090, got %d", cfg.PeerPort)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel to be 'debug', got '%s'", cfg.LogLevel)
	}

	// Test Kafka brokers
	if len(cfg.KafkaBrokers) != 2 {
		t.Errorf("Expected 2 Kafka brokers, got %d", len(cfg.KafkaBrokers))
	}
	if cfg.KafkaBrokers[0] != "kafka1:9092" {
		t.Errorf("Expected first Kafka broker to be 'kafka1:9092', got '%s'", cfg.KafkaBrokers[0])
	}
	if cfg.KafkaBrokers[1] != "kafka2:9092" {
		t.Errorf("Expected second Kafka broker to be 'kafka2:9092', got '%s'", cfg.KafkaBrokers[1])
	}

	// Test listeners
	if len(cfg.Listeners) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(cfg.Listeners))
	}

	// Test first listener (ws)
	if cfg.Listeners[0].Network != "ws" {
		t.Errorf("Expected first listener network to be 'ws', got '%s'", cfg.Listeners[0].Network)
	}
	if cfg.Listeners[0].Port != 8081 {
		t.Errorf("Expected first listener port to be 8081, got %d", cfg.Listeners[0].Port)
	}
	if cfg.Listeners[0].QueueSize != 512 {
		t.Errorf("Expected first listener queueSize to be 512, got %d", cfg.Listeners[0].QueueSize)
	}
	if !cfg.Listeners[0].AutoStart {
		t.Errorf("Expected first listener autoStart to be true, got false")
	}
	if cfg.Listeners[0].TLS != nil {
		t.Errorf("Expected first listener TLS to be nil, got %v", cfg.Listeners[0].TLS)
	}

	// Test second listener (wss with TLS)
	if cfg.Listeners[1].Network != "wss" {
		t.Errorf("Expected second listener network to be 'wss', got '%s'", cfg.Listeners[1].Network)
	}
	if cfg.Listeners[1].Port != 8082 {
		t.Errorf("Expected second listener port to be 8082, got %d", cfg.Listeners[1].Port)
	}
	if cfg.Listeners[1].QueueSize != 1024 {
		t.Errorf("Expected second listener queueSize to be 1024, got %d", cfg.Listeners[1].QueueSize)
	}
	if cfg.Listeners[1].AutoStart {
		t.Errorf("Expected second listener autoStart to be false, got true")
	}
	if cfg.Listeners[1].TLS == nil {
		t.Errorf("Expected second listener TLS to not be nil")
	} else {
		if cfg.Listeners[1].TLS.CertFile != "test.crt" {
			t.Errorf("Expected TLS certFile to be 'test.crt', got '%s'", cfg.Listeners[1].TLS.CertFile)
		}
		if cfg.Listeners[1].TLS.KeyFile != "test.key" {
			t.Errorf("Expected TLS keyFile to be 'test.key', got '%s'", cfg.Listeners[1].TLS.KeyFile)
		}
	}
}

func TestConfig_Level(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
		expected string
	}{
		{"debug level", "debug", "DEBUG"},
		{"info level", "info", "INFO"},
		{"warn level", "warn", "WARN"},
		{"error level", "error", "ERROR"},
		{"default level", "invalid", "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config{
				LogLevel: tt.logLevel,
			}
			level := cfg.Level()
			if level.String() != tt.expected {
				t.Errorf("Expected log level %s, got %s for input %s", tt.expected, level.String(), tt.logLevel)
			}
		})
	}
}

func TestYamlMarshalUnmarshal(t *testing.T) {
	// Test that yaml marshal/unmarshal works correctly with our structs
	originalCfg := &config{
		Redis: redisConfig{
			DB:       0,
			Address:  "localhost:6379",
			Password: "",
		},
		Pprof:        true,
		BrokerID:     1,
		InitSize:     5,
		HTTPPort:     8888,
		PeerPort:     4567,
		LogLevel:     "info",
		KafkaBrokers: []string{"localhost:9092"},
		MessageRoute: map[packet.MessageKind]route{
			packet.MessageKindToUser: {
				ResendType: resendToUser,
				KafkaTopic: "user-topic",
			},
			packet.MessageKindToChannel: {
				ResendType: resendToChannel,
				KafkaTopic: "group-topic",
			},
		},
		Listeners: []listener{
			{
				Port:      1881,
				Network:   "ws",
				QueueSize: 1024,
				AutoStart: true,
			},
		},
	}

	// Marshal to yaml
	data, err := yaml.Marshal(originalCfg)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Unmarshal back
	var unmarshaledCfg config
	if err := yaml.Unmarshal(data, &unmarshaledCfg); err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify the results
	if unmarshaledCfg.Redis.Address != originalCfg.Redis.Address {
		t.Errorf("Expected Redis Address to be %s, got %s", originalCfg.Redis.Address, unmarshaledCfg.Redis.Address)
	}
	if unmarshaledCfg.Pprof != originalCfg.Pprof {
		t.Errorf("Expected Pprof to be %v, got %v", originalCfg.Pprof, unmarshaledCfg.Pprof)
	}
	if len(unmarshaledCfg.Listeners) != len(originalCfg.Listeners) {
		t.Errorf("Expected %d listeners, got %d", len(originalCfg.Listeners), len(unmarshaledCfg.Listeners))
	}
}

func TestInvalidConfigFile(t *testing.T) {
	// Test reading an invalid config file
	invalidConfig := `
redis:
  db: "not-a-number"  # This should fail
  address: "localhost:6379"
`

	// Create temporary invalid config file
	tempFile, err := os.CreateTemp("", "invalid-config-*.yaml")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.WriteString(invalidConfig); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("Failed to close temp file: %v", err)
	}

	// Reading invalid config should fail
	_, err = readConfig(tempFile.Name())
	if err == nil {
		t.Error("Expected reading invalid config to fail, but it succeeded")
	}
}
