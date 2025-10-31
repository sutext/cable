package broker

type Config struct {
	Peer  PeerConfig
	Addr  string
	Redis RedisConfig
}

type RedisConfig struct {
	DB       int
	Addr     string
	Username string
	Password string
}

type PeerConfig struct {
	Addr string
}

func DefaultConfig() *Config {
	return &Config{
		Peer: PeerConfig{
			Addr: ":9090",
		},
		Addr: ":8080",
		Redis: RedisConfig{
			Addr:     "localhost:6379",
			Username: "",
			Password: "",
			DB:       0,
		},
	}
}
