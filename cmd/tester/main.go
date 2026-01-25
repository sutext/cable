package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"time"

	"sutext.github.io/cable/client"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type SendType string

const (
	SendTypeUser    SendType = "user"
	SendTypeChannel SendType = "channel"
)

type config struct {
	LogLevel        string                          `json:"log_level"`
	MaxConns        int                             `json:"max_conns"`  // max clients will be created
	ConnSpeed       int                             `json:"conn_speed"` // conn speed per second
	LogFormat       string                          `json:"log_format"`
	Endpoints       []string                        `json:"endpoints"`
	MaxUserID       int                             `json:"max_user_id"`
	MaxChannelID    int                             `json:"max_channel_id"`
	MessageInterval int                             `json:"message_interval"` // message speed per second
	MessageRoute    map[packet.MessageKind]SendType `json:"message_route"`
}

func readConfig(path string) (*config, error) {
	// Read config file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Parse yaml to config struct
	cfg := &config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Validate config
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}
func (c *config) validate() error {
	if c.LogLevel != "debug" && c.LogLevel != "info" && c.LogLevel != "warn" && c.LogLevel != "error" {
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return fmt.Errorf("invalid log format: %s", c.LogFormat)
	}
	if c.MaxConns <= 0 {
		return fmt.Errorf("invalid max conns: %d", c.MaxConns)
	}
	if c.ConnSpeed <= 0 {
		return fmt.Errorf("invalid conn speed: %d", c.ConnSpeed)
	}
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("invalid endpoints: %v", c.Endpoints)
	}
	if c.MaxUserID <= 0 {
		return fmt.Errorf("invalid max user id: %d", c.MaxUserID)
	}
	if c.MaxChannelID <= 0 {
		return fmt.Errorf("invalid max channel id: %d", c.MaxChannelID)
	}
	if len(c.MessageRoute) == 0 {
		return fmt.Errorf("invalid message route: %v", c.MessageRoute)
	}
	for k, v := range c.MessageRoute {
		if k > 63 {
			return fmt.Errorf("invalid message kind: %d", k)
		}
		if v != SendTypeUser && v != SendTypeChannel {
			return fmt.Errorf("invalid send type: %s", v)
		}
	}
	if c.MessageInterval <= 0 {
		return fmt.Errorf("invalid message speed: %d", c.MessageInterval)
	}
	return nil
}
func (c *config) Level() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
func (c *config) randomUserID() string {
	return fmt.Sprintf("u%d", rand.IntN(c.MaxUserID))
}
func (c *config) randomChannelID() string {
	return fmt.Sprintf("channel%d", rand.IntN(c.MaxChannelID))
}
func (t *config) randomEndpoint() string {
	if len(t.Endpoints) == 1 {
		return t.Endpoints[0]
	}
	return t.Endpoints[rand.IntN(len(t.Endpoints))]
}
func (t *config) randomKind() packet.MessageKind {
	keys := make([]packet.MessageKind, 0, len(t.MessageRoute))
	for kind := range t.MessageRoute {
		keys = append(keys, kind)
	}
	return keys[rand.IntN(len(keys))]
}
func (t *config) randomMessage() *packet.Message {
	kind := t.randomKind()
	sendType := t.MessageRoute[kind]
	content := fmt.Sprintf("hello this message is send to %s:", sendType)
	msg := packet.NewMessage([]byte(content))
	msg.Kind = kind
	switch sendType {
	case SendTypeUser:
		msg.Set(packet.PropertyUserID, t.randomUserID())
	case SendTypeChannel:
		msg.Set(packet.PropertyChannel, t.randomChannelID())
	default:
	}
	return msg
}

type Client struct {
	config *config
	cli    client.Client
}

func newClient(config *config) *Client {
	result := &Client{config: config}
	uid := config.randomUserID()
	cid := fmt.Sprintf("%d-%d", time.Now().UnixNano(), rand.Int64())
	result.cli = client.New(config.randomEndpoint(),
		client.WithID(&packet.Identity{UserID: uid, ClientID: cid}),
		client.WithKeepAlive(time.Second*5, time.Second*60),
		client.WithHandler(result),
	)
	return result
}
func (c *Client) ID() *packet.Identity {
	return c.cli.ID()
}
func (c *Client) Connect() {
	c.cli.Connect()
}
func (c *Client) run() {
	for {
		if !c.cli.IsReady() {
			return
		}
		msg := c.config.randomMessage()
		if msg == nil {
			continue
		}
		err := c.cli.SendMessage(context.Background(), msg)
		if err != nil {
			xlog.Error("send message error",
				xlog.Int("Kind", int(msg.Kind)),
				xlog.Err(err),
			)
		} else {
			xlog.Info("send message success",
				xlog.Int("Kind", int(msg.Kind)),
			)
		}
		time.Sleep(time.Second * time.Duration(c.config.MessageInterval))
	}
}

func (c *Client) OnStatus(status client.Status) {
	if c.cli.IsReady() {
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

var configFile = "config.json"

func init() {
	flag.StringVar(&configFile, "config", "config.json", "config file path")
}
func main() {
	flag.Parse()
	conf, err := readConfig(configFile)
	if err != nil {
		panic(err)
	}
	var logger *xlog.Logger
	if conf.LogFormat == "json" {
		logger = xlog.NewJSON(conf.Level().Level())
	} else {
		logger = xlog.NewText(conf.Level().Level())
	}
	var clients safe.RMap[string, *Client]
	xlog.SetDefault(logger)
	ctx := context.Background()
	dur := time.Second * time.Duration(conf.MaxConns/conf.ConnSpeed)
	ctx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()
	go func() {
		ticker := time.NewTicker(time.Second / time.Duration(conf.ConnSpeed))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c := newClient(conf)
				clients.Set(c.ID().ClientID, c)
				go c.Connect()
			}
		}
	}()
	select {}
}
