package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
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
}
func (c *Client) OnRequest(req *packet.Request) (*packet.Response, error) {
	return nil, fmt.Errorf("unsupported request")
}
func RandomClient() *Client {
	result := &Client{}
	result.cli = client.New("172.16.2.123:1883",
		client.WithNetwork(client.NewworkTCP),
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithHandler(result),
	)
	uid := fmt.Sprintf("u%d", rand.IntN(300))
	cid := fmt.Sprintf("%s_%d", uid, rand.Int())
	result.id = &packet.Identity{UserID: uid, ClientID: cid}
	result.backoff = backoff.Random(time.Second*3, time.Second*10)
	return result
}

func main() {
	xlog.SetDefault(xlog.WithLevel(xlog.LevelInfo))
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Second*70)
	var clients sync.Map
	defer cancel()
	go func() {
		ticker := time.NewTicker(time.Millisecond * 10)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c := RandomClient()
				clients.Store(c.id.ClientID, c)
				go c.Connect()
			}
		}
	}()
	select {}
}
