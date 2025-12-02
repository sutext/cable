package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"sutext.github.io/cable/client"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Client struct {
	id  *packet.Identity
	cli client.Client
}

func (c *Client) run() {
	for {
		if c.cli.Status() == client.StatusClosed {
			return
		}
		switch rand.Int() % 3 {
		case 0:
			c.sendToAll()
		case 1:
			c.sendToUser()
		case 2:
			c.sendToChannel()
		}
		time.Sleep(time.Second * time.Duration(100+rand.IntN(10)))
	}
}

func (c *Client) sendToUser() {
	enc := coder.NewEncoder()
	toUserID := fmt.Sprintf("u%d", rand.IntN(100000))
	enc.WriteString(toUserID)
	enc.WriteString(fmt.Sprintf("hello i am %s", c.id.UserID))
	msg := packet.NewMessage(enc.Bytes())
	msg.Qos = packet.MessageQos1
	msg.Kind = packet.MessageKind(1)
	err := c.cli.SendMessage(context.Background(), msg)
	if err != nil {
		xlog.Error("sendToUser message error", err)
	}
}
func (c *Client) sendToChannel() {
	enc := coder.NewEncoder()
	channel := fmt.Sprintf("channel%d", rand.IntN(100000))
	enc.WriteString(channel)
	enc.WriteString(fmt.Sprintf("hello group, i am %s", c.id.UserID))
	msg := packet.NewMessage(enc.Bytes())
	msg.Qos = packet.MessageQos1
	// msg.Kind = packet.MessageKind(2)
	err := c.cli.SendMessage(context.Background(), msg)
	if err != nil {
		xlog.Error("sendToChannel message error", err)
	}
}
func (c *Client) sendToAll() {
	msg := packet.NewMessage(fmt.Appendf(nil, "hello every one, i am %s", c.id.UserID))
	// msg.Kind = packet.MessageKind(3)
	err := c.cli.SendMessage(context.Background(), msg)
	if err != nil {
		xlog.Error("sendToAll message error", err)
	}
}
func (c *Client) OnStatus(status client.Status) {
	if status == client.StatusOpened {
		xlog.Info("client opened")
		go c.run()
	}
}
func (c *Client) OnMessage(msg *packet.Message) error {
	return nil
}
func (c *Client) OnRequest(req *packet.Request) (*packet.Response, error) {
	return nil, fmt.Errorf("unsupported request")
}

type Tester struct {
	cond       *sync.Cond
	paused     bool
	clients    safe.Map[string, *Client]
	httpServer *http.Server
}

func NewTester() *Tester {
	t := &Tester{
		cond: sync.NewCond(&sync.Mutex{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/pause", t.handlePause)
	t.httpServer = &http.Server{Addr: ":6666", Handler: mux}
	return t
}
func (t *Tester) Run() {
	for {
		t.cond.L.Lock()
		for t.paused {
			t.cond.Wait()
		}
		t.cond.L.Unlock()
		t.runOnce()
		time.Sleep(time.Second * time.Duration(20+rand.IntN(10)))
	}
}
func (t *Tester) addClient() *Client {
	result := &Client{}
	result.cli = client.New("cable-cluster:1883",
		client.WithNetwork(client.NewworkTCP),
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithHandler(result),
	)
	uid := fmt.Sprintf("u%d", rand.IntN(100000))
	cid := fmt.Sprintf("%s_%d", uid, rand.Int())
	result.id = &packet.Identity{UserID: uid, ClientID: cid}
	t.clients.Set(result.id.ClientID, result)
	result.cli.Connect(result.id)
	return result
}
func (t *Tester) handlePause(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("pause")
	if p == "true" {
		t.Pause()
	} else {
		t.Resume()
	}
	w.Write([]byte("ok"))
}
func (t *Tester) runOnce() {
	max := 100
	xlog.Info("OK we pick 100 clients to send message")
	t.clients.Range(func(key string, value *Client) bool {
		if value.cli.Status() == client.StatusOpened {
			max--
			value.sendToUser()
		}
		return max > 0
	})
}
func (t *Tester) Pause() {
	t.cond.L.Lock()
	t.paused = true
	t.cond.L.Unlock()
}
func (t *Tester) Resume() {
	t.cond.L.Lock()
	t.paused = false
	t.cond.L.Unlock()
	t.cond.Signal()
}
