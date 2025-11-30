package main

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"sutext.github.io/cable/backoff"
	"sutext.github.io/cable/client"
	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/internal/safe"
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
	xlog.Info("receive message", slog.String("msg", string(msg.Payload)))
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
		time.Sleep(time.Second * time.Duration(10+rand.IntN(10)))
	}
}
func (t *Tester) addClient() *Client {
	result := &Client{}
	result.cli = client.New("cable-cluster:1883",
		client.WithNetwork(client.NewworkTCP),
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithHandler(result),
	)
	uid := fmt.Sprintf("u%d", rand.IntN(30000))
	cid := fmt.Sprintf("%s_%d", uid, rand.Int())
	result.id = &packet.Identity{UserID: uid, ClientID: cid}
	result.backoff = backoff.Random(time.Second*3, time.Second*10)
	t.clients.Set(result.id.ClientID, result)
	result.Connect()
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
			enc := coder.NewEncoder()
			toUserID := fmt.Sprintf("u%d", rand.IntN(30000))
			enc.WriteString(toUserID)
			enc.WriteString(fmt.Sprintf("hello i am %s", value.id.UserID))
			msg := packet.NewMessage(enc.Bytes())
			msg.Qos = packet.MessageQos1
			msg.Kind = packet.MessageKind(1)
			err := value.cli.SendMessage(context.Background(), msg)
			if err != nil {
				fmt.Println(err)
			}
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
