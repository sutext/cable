package main

import (
	"fmt"
	"math/rand/v2"
	"time"

	"sutext.github.io/cable/client"
	"sutext.github.io/cable/internal/backoff"
	"sutext.github.io/cable/packet"
)

type Client struct {
	cli     *client.Client
	userId  string
	token   string
	backoff backoff.Backoff
	count   int
}

func RandomClient() *Client {
	return &Client{
		cli:     client.New("localhost", "8080"),
		userId:  fmt.Sprintf("user_%d", rand.Int()),
		token:   fmt.Sprintf("access_token_%d", rand.Int()),
		backoff: backoff.Random(time.Second*5, time.Second*10),
		count:   0,
	}
}
func (c *Client) Start() {
	c.cli.Connect(&packet.Identity{AuthToken: c.userId, UserID: c.userId, ClientID: c.token})
	for {
		c.cli.SendData([]byte("hello world"))
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
	addClient(1000)
	select {}
}
