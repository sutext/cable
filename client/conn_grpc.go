package client

import (
	"context"
	"log"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"sutext.github.io/cable/internal/listener/pb"
	"sutext.github.io/cable/packet"
)

type grpcConn struct {
	addr   string
	conn   *grpc.ClientConn
	stream grpc.BidiStreamingClient[pb.Bytes, pb.Bytes]
}

func newGRPCConn(addr string) Conn {
	return &grpcConn{addr: addr}
}
func (c *grpcConn) Dail() error {
	conn, err := grpc.NewClient(c.addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	if c.conn != nil {
		c.conn.Close()
	}
	c.conn = conn
	cli := pb.NewBytesServiceClient(conn)
	s, err := cli.Connect(context.Background())
	if err != nil {
		return err
	}
	c.stream = s
	return nil
}
func (c *grpcConn) Close() error {
	return c.conn.Close()
}
func (c *grpcConn) ReadPacket() (packet.Packet, error) {
	b, err := c.stream.Recv()
	if err != nil {
		return nil, err
	}
	return packet.Unmarshal(b.Data)
}
func (c *grpcConn) WritePacket(p packet.Packet) error {
	b, err := packet.Marshal(p)
	if err != nil {
		return err
	}
	return c.stream.Send(&pb.Bytes{Data: b})
}
