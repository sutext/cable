package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"

	"sutext.github.io/cable/api"
	"sutext.github.io/cable/xlog"
)

var (
	client   api.Client
	endpoint string
	cmd      string
	uid      string
	network  string
	chs      map[string]string
)

func init() {
	flag.StringVar(&endpoint, "endpoint", "localhost:1887", "endpoint of cable server")
	flag.StringVar(&cmd, "cmd", "join", "sub command")
	flag.StringVar(&uid, "uid", "", "uid of user")
	flag.StringVar(&network, "network", "tcp", "network of listener")
}
func main() {
	flag.Parse()
	client = api.NewClient(endpoint)
	client.Connect()
	switch cmd {
	case "startListener":
		err := client.StartListener(context.Background(), network)
		if err != nil {
			xlog.Error("start listener failed", xlog.Str("network", network), xlog.Err(err))
		} else {
			xlog.Info("start listener success", xlog.Str("network", network))
		}
	case "stopListener":
		err := client.StopListener(context.Background(), network)
		if err != nil {
			xlog.Error("stop listener failed", xlog.Str("network", network), xlog.Err(err))
		} else {
			xlog.Info("stop listener success", xlog.Str("network", network))
		}
	case "join":
		if uid == "" {
			xlog.Error("uid is empty")
			return
		}
		err := client.JoinChannel(context.Background(), uid, chs)
		if err != nil {
			xlog.Error("join channel failed", xlog.Str("uid", uid), xlog.Err(err))
		} else {
			xlog.Info("join channel success", xlog.Str("uid", uid))
		}
	case "leave":
		if uid == "" {
			xlog.Error("uid is empty")
			return
		}
		err := client.LeaveChannel(context.Background(), uid, chs)
		if err != nil {
			xlog.Error("leave channel failed", xlog.Str("uid", uid), xlog.Err(err))
		} else {
			xlog.Info("leave channel success", xlog.Str("uid", uid))
		}
	case "list":
		if uid == "" {
			xlog.Error("uid is empty")
			return
		}
		channels, err := client.ListChannels(context.Background(), uid)
		if err != nil {
			xlog.Error("list channel failed", xlog.Str("uid", uid), xlog.Err(err))
		} else {
			xlog.Info("list channel success", xlog.Str("uid", uid), xlog.Any("channels", channels))
		}
	default:
		xlog.Error("unknown command", xlog.Str("cmd", cmd))
	}

}

func RandomJoin() {
	ctx := context.Background()
	max := 100000
	for i := range max {
		uid := fmt.Sprintf("u%d", i)
		err := client.JoinChannel(ctx, uid, randomChannelIDs(max))
		if err != nil {
			xlog.Error("join channel failed", xlog.Str("uid", uid), xlog.Err(err))
		} else {
			xlog.Info("join channel success", xlog.Str("uid", uid))
		}
	}
}
func randomChannelIDs(max int) map[string]string {
	ch1 := fmt.Sprintf("channel%d", rand.IntN(max))
	ch2 := fmt.Sprintf("channel%d", rand.IntN(max))
	// ch3 := fmt.Sprintf("channel%d", rand.IntN(max))
	return map[string]string{
		ch1: "",
		ch2: "",
		// ch3: "",
	}
}
