package main

import (
	"context"
	"fmt"
	"net"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"sutext.github.io/cable/discovery"
	"sutext.github.io/cable/xlog"
)

type etcdDiscovery struct {
	id        uint64
	ipaddr    string
	logger    *xlog.Logger
	endpoints []string
	client    *clientv3.Client
	handler   discovery.Handler
	leaseID   clientv3.LeaseID
	stopChan  chan struct{}
	ctx       context.Context
	cancel    context.CancelFunc
}

func newEtcdDiscovery(id uint64, port uint16, logger *xlog.Logger, endpoints []string) *etcdDiscovery {
	ip, err := getLocalIP()
	if err != nil {
		panic(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &etcdDiscovery{
		id:        id,
		ipaddr:    fmt.Sprintf("%s:%d", ip, port),
		logger:    logger,
		endpoints: endpoints,
		stopChan:  make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (e *etcdDiscovery) Start() {
	// Connect to etcd
	cfg := clientv3.Config{
		Endpoints:   e.endpoints,
		DialTimeout: 5 * time.Second,
	}
	client, err := clientv3.New(cfg)
	if err != nil {
		e.logger.Error("failed to create etcd client", xlog.Err(err))
		return
	}
	e.client = client
	// Create a lease with 10-second TTL
	lease, err := client.Grant(context.Background(), 10)
	if err != nil {
		e.logger.Error("failed to create lease", xlog.Err(err))
		return
	}
	e.leaseID = lease.ID
	// Register this node in etcd
	nodeKey := fmt.Sprintf("/cable/nodes/%d", e.id)
	_, err = client.Put(context.Background(), nodeKey, e.ipaddr, clientv3.WithLease(e.leaseID))
	if err != nil {
		e.logger.Error("failed to register node", xlog.Err(err))
		return
	}
	e.logger.Info("etcd node registered", xlog.Str("key", nodeKey), xlog.Str("value", e.ipaddr))
	go e.keepaliveLeaseLoop()
	go e.watchNodesLoop()
	e.discoverExistingNodes()
}

func (e *etcdDiscovery) keepaliveLeaseLoop() {
	if e.client == nil {
		return
	}
	keepAliveChan, err := e.client.KeepAlive(e.ctx, e.leaseID)
	if err != nil {
		e.logger.Error("failed to start lease keep-alive", xlog.Err(err))
		return
	}
	for {
		select {
		case <-e.stopChan:
			return
		case <-e.ctx.Done():
			return
		case ka := <-keepAliveChan:
			if ka == nil {
				e.logger.Error("lease keep-alive channel closed")
				return
			}
		}
	}
}

func (e *etcdDiscovery) watchNodesLoop() {
	if e.client == nil {
		return
	}
	watchChan := e.client.Watch(e.ctx, "/cable/nodes/", clientv3.WithPrefix())
	for {
		select {
		case <-e.stopChan:
			return
		case <-e.ctx.Done():
			return
		case watchResp := <-watchChan:
			if watchResp.Err() != nil {
				e.logger.Error("watch error", xlog.Err(watchResp.Err()))
				return
			}
			for _, event := range watchResp.Events {
				switch event.Type {
				case clientv3.EventTypePut:
					key := string(event.Kv.Key)
					value := string(event.Kv.Value)
					nodeID, err := extractNodeID(key)
					if err != nil {
						e.logger.Error("failed to extract node ID from key", xlog.Str("key", key), xlog.Err(err))
						continue
					}
					if e.handler != nil {
						e.handler.OnNodeJoin(nodeID, value)
						e.logger.Info("node joined", xlog.U64("nodeID", nodeID), xlog.Str("addr", value))
					}
				case clientv3.EventTypeDelete:
					key := string(event.Kv.Key)
					nodeID, err := extractNodeID(key)
					if err != nil {
						e.logger.Error("failed to extract node ID from key", xlog.Str("key", key), xlog.Err(err))
						continue
					}
					if e.handler != nil {
						e.handler.OnNodeLeave(nodeID)
						e.logger.Info("node left", xlog.U64("nodeID", nodeID))
					}
				}
			}
		}
	}
}

func (e *etcdDiscovery) discoverExistingNodes() {
	if e.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := e.client.Get(ctx, "/cable/nodes/", clientv3.WithPrefix())
	if err != nil {
		e.logger.Error("failed to discover existing nodes", xlog.Err(err))
		return
	}
	for _, kv := range resp.Kvs {
		key := string(kv.Key)
		value := string(kv.Value)
		nodeID, err := extractNodeID(key)
		if err != nil {
			e.logger.Error("failed to extract node ID from key", xlog.Str("key", key), xlog.Err(err))
			continue
		}
		// Don't trigger handler for our own node
		if nodeID != e.id && e.handler != nil {
			e.handler.OnNodeJoin(nodeID, value)
			e.logger.Info("discovered existing node", xlog.U64("nodeID", nodeID), xlog.Str("addr", value))
		}
	}
}

func (e *etcdDiscovery) Shutdown() error {
	e.cancel()
	close(e.stopChan)
	if e.client != nil {
		// Revoke the lease to remove the node registration
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		e.client.Revoke(ctx, e.leaseID)
		return e.client.Close()
	}
	return nil
}

func (e *etcdDiscovery) SetHandler(handler discovery.Handler) {
	e.handler = handler
}

// getLocalIP retrieves the non-loopback local IPv4 address of the machine.
func getLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		// Check if the interface is up and not a loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no network interface found")
}

// extractNodeID extracts the node ID from an etcd key like "/cable/nodes/{nodeId}"
func extractNodeID(key string) (uint64, error) {
	// Expected format: /cable/nodes/{nodeId}
	// Parse the last part as nodeId
	var nodeID uint64
	_, err := fmt.Sscanf(key, "/cable/nodes/%d", &nodeID)
	if err != nil {
		return 0, fmt.Errorf("failed to parse node ID from key %s: %w", key, err)
	}
	return nodeID, nil
}
