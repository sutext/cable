rm -rf ./cluster/pb/*.pb.go 
protoc --go_out=./cluster/ --go-grpc_out=./cluster/ ./cluster/pb/peer.proto
rm -rf ./api/pb/*.pb.go 
protoc --go_out=./api/ --go-grpc_out=./api/ ./api/pb/cable.proto
