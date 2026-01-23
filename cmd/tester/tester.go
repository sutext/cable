package main

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"sutext.github.io/cable/client"
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
		if !c.cli.IsReady() {
			return
		}
		switch rand.Int() % 2 {
		case 0:
			c.sendToUser()
		case 1:
			c.sendToChannel()
		}
		time.Sleep(time.Second * 2)
	}
}

func (c *Client) sendToUser() {
	msg := packet.NewMessage(fmt.Appendf(nil, "hello, i am %s", c.id.UserID))
	msg.Qos = packet.MessageQos1
	msg.Kind = packet.MessageKind(1)
	msg.Set(packet.PropertyUserID, randomUserID())
	err := c.cli.SendMessage(context.Background(), msg)
	if err != nil {
		xlog.Error("sendToChannel message error", xlog.Err(err))
	}
}
func (c *Client) sendToChannel() {
	msg := packet.NewMessage(fmt.Appendf(nil, "hello group, i am %s", c.id.UserID))
	msg.Qos = packet.MessageQos1
	msg.Kind = packet.MessageKind(2)
	msg.Set(packet.PropertyChannel, randomChannel())
	err := c.cli.SendMessage(context.Background(), msg)
	if err != nil {
		xlog.Error("sendToChannel message error", xlog.Err(err))
	}
}
func randomUserID() string {
	return fmt.Sprintf("u%d", rand.IntN(100000))
}
func randomClientID() string {
	return fmt.Sprintf("c%d", rand.Int64())
}
func randomChannel() string {
	return fmt.Sprintf("channel%d", rand.IntN(100000))
}
func (c *Client) OnStatus(status client.Status) {
	if c.cli.IsReady() {
		xlog.Info("client opened")
		go c.run()
	}
}
func (c *Client) OnMessage(msg *packet.Message) error {
	xlog.Info("receive message", xlog.Msg(string(msg.Payload)))
	return nil
}
func (c *Client) OnRequest(req *packet.Request) (*packet.Response, error) {
	return nil, fmt.Errorf("unsupported request")
}

type Tester struct {
	cond       *sync.Cond
	paused     bool
	clients    safe.RMap[string, *Client]
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
func statefulSetEndpoint() string {
	appName := os.Getenv("APP_NAME")
	if appName == "" {
		return "cable-cluster"
	}
	appCount := os.Getenv("APP_COUNT")
	if appCount == "" {
		appCount = "5"
	}
	count, err := strconv.ParseInt(appCount, 10, 64)
	if err != nil || count < 1 {
		panic("invalid APP_COUNT")
	}
	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "1883"
	}
	service := os.Getenv("APP_SERVICE")
	if service == "" {
		service = "cable"
	}
	network := os.Getenv("APP_PROTO")
	if network == "" {
		network = "tcp"
	}
	return fmt.Sprintf("%s://%s-%d.%s:%s", network, appName, rand.Int64()%count, service, port)
}
func (t *Tester) addClient() *Client {
	endpoint := os.Getenv("ENDPOINT")
	if endpoint == "" {
		endpoint = statefulSetEndpoint()
	}
	result := &Client{}
	result.cli = client.New(endpoint,
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithHandler(result),
	)
	result.id = &packet.Identity{UserID: randomUserID(), ClientID: randomClientID()}
	t.clients.Set(result.id.ClientID, result)
	go result.cli.Connect(result.id)
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
		if value.cli.IsReady() {
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
