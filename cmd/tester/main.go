package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"sync"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Client struct {
	id      *packet.Identity
	cli     client.Client
	backoff backoff.Backoff
}

func (c *Client) Connect() {
	c.cli.Connect(c.id)
}
func (c *Client) OnStatus(status client.Status) {

}
func (c *Client) OnMessage(msg *packet.Message) error {
	return nil
	// fmt.Printf("client %s receive message: %s\n", c.id.UserID, string(msg.Payload))
}
func (c *Client) OnRequest(req *packet.Request) (*packet.Response, error) {
	return nil, fmt.Errorf("unsupported request")
}
func RandomClient() *Client {
	result := &Client{}
	addrs := []string{"127.0.0.1:8080", "127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"}
	// addrs := []string{"127.0.0.1:8080"}
	// addrs := []string{"172.16.2.123:8080", "172.16.2.123:8081", "172.16.2.123:8082", "172.16.2.123:8083"}
	result.cli = client.New(addrs[rand.IntN(len(addrs))],
		client.WithNetwork(client.NewworkTCP),
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithRetry(math.MaxInt, backoff.Exponential(2, 2)),
		client.WithHandler(result),
		client.WithLogger(logger.NewText(slog.LevelDebug)),
	)
	uid := fmt.Sprintf("u%d", rand.IntN(300))
	cid := fmt.Sprintf("%s_%d", uid, rand.Int())
	result.id = &packet.Identity{UserID: uid, ClientID: cid}
	result.backoff = backoff.Random(time.Second*3, time.Second*10)
	return result
}

var clients sync.Map

func addClient(count uint) {
	for range count {
		c := RandomClient()
		clients.Store(c.id.ClientID, c)
		go c.Connect()
	}
}
func runBrokerClients() {
	ctx := context.Background()
	ticker := time.NewTicker(time.Millisecond * 1000)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			addClient(10)
		}
	}
}

func runServerClietn() {
	c := RandomClient()
	clients.Store(c.id.ClientID, c)
	c.Connect()
}
func main() {
	runBrokerClients()
	// runServerClietn()
	select {}
}
