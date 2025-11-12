package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/internal/logger"
	"sutext.github.io/cable/packet"
)

type Client struct {
	id      *packet.Identity
	cli     client.Client
	count   int
	backoff backoff.Backoff
}

func (c *Client) OnStatus(status client.Status) {

}
func (c *Client) OnMessage(msg *packet.Message) {
	// fmt.Printf("client %s receive message: %s\n", c.id.UserID, string(msg.Payload))
}
func (c *Client) OnRequest(req *packet.Request) (*packet.Response, error) {
	return nil, fmt.Errorf("unsupported request")
}
func RandomClient() *Client {
	result := &Client{}
	addrs := []string{"127.0.0.1:8080", "127.0.0.1:8081", "127.0.0.1:8082", "127.0.0.1:8083"}
	// addrs := []string{"172.16.2.123:8080", "172.16.2.123:8081", "172.16.2.123:8082", "172.16.2.123:8083"}
	result.cli = client.New(addrs[rand.IntN(len(addrs))],
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithRetry(math.MaxInt, backoff.Exponential(2, 2)),
		client.WithHandler(result),
		client.WithLogger(logger.NewText(slog.LevelError)),
	)
	result.id = &packet.Identity{UserID: fmt.Sprintf("u%d", rand.Int()), ClientID: fmt.Sprintf("c%d", rand.Int())}
	result.backoff = backoff.Random(time.Second*3, time.Second*10)
	return result
}

var channels = []string{"news", "sports", "tech", "music", "movies"}

func (c *Client) Start() {
	c.cli.Connect(c.id)
	for {
		channel := channels[rand.IntN(len(channels))]
		msg := packet.NewMessage(fmt.Appendf(nil, "hello im client %s", c.id.UserID))
		msg.Channel = channel
		err := c.cli.SendMessage(msg)
		if err != nil {
			fmt.Printf("client %s send message error: %v\n", c.id.UserID, err)
		}
		time.Sleep(c.backoff.Next(c.count))
		c.count++
	}
}
func addClient(count uint) {
	for range count {
		c := RandomClient()
		go c.Start()
	}
}

func main() {
	ctx := context.Background()
	ticker := time.NewTicker(time.Second * 1)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			addClient(100)
		}
	}
}
