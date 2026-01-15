module cable

go 1.25.2

replace sutext.github.io/cable => ../../

require (
	github.com/redis/go-redis/v9 v9.17.2
	github.com/segmentio/kafka-go v0.2.2
	golang.org/x/net v0.46.1-0.20251013234738-63d1a5100f82
	gopkg.in/yaml.v3 v3.0.1
	sutext.github.io/cable v0.0.0-00010101000000-000000000000
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	go.etcd.io/raft/v3 v3.6.0 // indirect
	golang.org/x/crypto v0.43.0 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251022142026-3a174f9686a8 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)
