rm -rf ./cluster/pb/*.pb.go 
protoc --go_out=./cluster/ --go-grpc_out=./cluster/ ./cluster/pb/peer.proto