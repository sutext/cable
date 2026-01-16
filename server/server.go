// Package server provides a server implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like connection management, broadcasting, and request-response mechanism.
package server

import (
	"context"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/network"
	"sutext.github.io/cable/internal/safe"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xerr"
	"sutext.github.io/cable/xlog"
)

// ConnInfo contains information about a client connection.
type ConnInfo struct {
	IP  string // IP address of the client
	UID string // User ID associated with the client
	CID string // Client ID of the connection
}

// Server defines the interface for the cable server.
type Server interface {
	// Serve starts the server and listens for incoming connections.
	Serve() error
	// Network returns the network protocol used by the server.
	Network() string
	/// IsActive returns true if the client with the given CID is active.
	IsActive(cid string) bool
	// ConnInfo returns the connection information of a client by CID.
	ConnInfo(cid string) (*ConnInfo, bool)
	// KickConn kicks a client out of the server by CID.
	KickConn(cid string) bool
	// Shutdown shuts down the server and closes all connections.
	Shutdown(ctx context.Context) error
	// Brodcast sends a message to all connected clients.
	Brodcast(ctx context.Context, p *packet.Message) (total, success int32, err error)
	// SendMessage sends a message to a specific client by CID.
	SendMessage(ctx context.Context, cid string, p *packet.Message) error
	// SendRequest sends a request to a client and returns the response.
	SendRequest(ctx context.Context, cid string, p *packet.Request) (*packet.Response, error)
	// ExpelAllConns expels all connections from the server.
	ExpelAllConns()
}

// server implements the Server interface.
type server struct {
	conns          safe.RMap[string, network.Conn] // Map of client connections (CID -> Conn)
	logger         *xlog.Logger                    // Logger instance
	closed         atomic.Bool                     // Closed flag (atomic)
	address        string                          // Server address to listen on
	network        string                          // Network protocol
	transport      network.Transport               // Network transport implementation
	closeHandler   ClosedHandler                   // Handler for connection close events
	connectHandler ConnectHandler                  // Handler for connection events
	messageHandler MessageHandler                  // Handler for message events
	requestHandler RequestHandler                  // Handler for request events
}

// New creates a new Server instance with the given address and options.
//
// Parameters:
// - address: Server address to listen on (e.g., ":8080")
// - opts: Configuration options for the server
//
// Returns:
// - Server: A new Server instance
func New(address string, opts ...Option) Server {
	options := NewOptions(opts...)
	s := &server{
		logger:         options.logger,
		address:        address,
		network:        options.network,
		closeHandler:   options.closeHandler,
		connectHandler: options.connectHandler,
		messageHandler: options.messageHandler,
		requestHandler: options.requestHandler,
	}
	// Create transport based on network protocol
	switch s.network {
	case NetworkWS:
		s.transport = network.NewWS(s.logger, options.queueCapacity)
	case NetworkWSS:
		s.transport = network.NewWSS(options.tlsConfig, s.logger, options.queueCapacity)
	case NetworkTCP:
		s.transport = network.NewTCP(s.logger, options.queueCapacity)
	case NetworkTLS:
		s.transport = network.NewTLS(options.tlsConfig, s.logger, options.queueCapacity)
	case NetworkUDP:
		s.transport = network.NewUDP(s.logger, options.queueCapacity)
	case NetworkQUIC:
		s.transport = network.NewQUIC(options.quicConfig, s.logger, options.queueCapacity)
	default:
		panic(xerr.NetworkNotSupported)
	}
	return s
}

// Serve starts the server and listens for incoming connections.
//
// Returns:
// - error: Error if the server fails to start, nil otherwise
func (s *server) Serve() error {
	s.transport.OnClose(s.onClose)
	s.transport.OnAccept(s.onConnect)
	s.transport.OnPacket(s.onPacket)
	s.logger.Info("server listening", xlog.Str("address", s.address), xlog.Str("network", string(s.network)))
	return s.transport.Listen(s.address)
}

// IsActive checks if a client connection is active by CID.
//
// Parameters:
// - cid: Client ID to check
//
// Returns:
// - bool: True if the connection is active, false otherwise
func (s *server) IsActive(cid string) bool {
	if c, ok := s.conns.Get(cid); ok {
		return !c.IsClosed()
	}
	return false
}

// Network returns the network protocol used by the server.
//
// Returns:
// - string: Network protocol name
func (s *server) Network() string {
	return s.network
}

// ConnInfo returns the connection information for a client by CID.
//
// Parameters:
// - cid: Client ID to get info for
//
// Returns:
// - *ConnInfo: Connection information if found, nil otherwise
// - bool: True if the connection exists, false otherwise
func (s *server) ConnInfo(cid string) (*ConnInfo, bool) {
	if c, ok := s.conns.Get(cid); ok {
		return &ConnInfo{
			IP:  c.IP(),
			UID: c.ID().UserID,
			CID: c.ID().ClientID,
		}, true
	}
	return nil, false
}

// KickConn kicks a client out of the server by CID.
//
// Parameters:
// - cid: Client ID to kick
//
// Returns:
// - bool: True if the client was kicked, false if not found
func (s *server) KickConn(cid string) bool {
	if c, ok := s.conns.Get(cid); ok {
		c.CloseClode(packet.CloseKickedOut)
		return true
	}
	return false
}

// ExpelAllConns expels all connections from the server.
func (s *server) ExpelAllConns() {
	s.conns.Range(func(key string, c network.Conn) bool {
		c.CloseClode(packet.CloseServerExpeled)
		return true
	})
	s.conns = safe.RMap[string, network.Conn]{}
}

// Shutdown shuts down the server and closes all connections.
//
// Parameters:
// - ctx: Context for the shutdown operation
//
// Returns:
// - error: Error if shutdown fails, nil otherwise
func (s *server) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return xerr.ServerAlreadyClosed
	}
	err := s.transport.Close(ctx)
	for {
		activeConn := 0
		s.conns.Range(func(key string, conn network.Conn) bool {
			if conn.IsIdle() {
				conn.Close()
			} else {
				activeConn++
			}
			return true
		})
		if activeConn == 0 { // all connections have been closed
			return err
		}
		waitTime := time.Millisecond * time.Duration(activeConn)
		if waitTime > time.Second { // max wait time is 1000 ms
			waitTime = time.Millisecond * 1000
		} else if waitTime < time.Millisecond*50 { // min wait time is 50 ms
			waitTime = time.Millisecond * 50
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			continue
		}
	}
}

// Brodcast sends a message to all connected clients.
//
// Parameters:
// - ctx: Context for the broadcast operation
// - p: Message packet to send
//
// Returns:
// - int32: Total number of clients attempted to send to
// - int32: Number of successful sends
// - error: Error if broadcast fails, nil otherwise
func (s *server) Brodcast(ctx context.Context, p *packet.Message) (int32, int32, error) {
	if s.closed.Load() {
		return 0, 0, xerr.ServerIsClosed
	}
	var total, success int32
	s.conns.Range(func(cid string, conn network.Conn) bool {
		total++
		if err := conn.SendMessage(ctx, p); err == nil {
			success++
		}
		return true
	})
	return total, success, nil
}

// SendMessage sends a message to a specific client by CID.
//
// Parameters:
// - ctx: Context for the send operation
// - cid: Client ID to send to
// - p: Message packet to send
//
// Returns:
// - error: Error if send fails, nil otherwise
func (s *server) SendMessage(ctx context.Context, cid string, p *packet.Message) error {
	if s.closed.Load() {
		return xerr.ServerIsClosed
	}
	if c, ok := s.conns.Get(cid); ok {
		return c.SendMessage(ctx, p)
	}
	return xerr.ConnectionNotFound
}

// SendRequest sends a request to a client and returns the response.
//
// Parameters:
// - ctx: Context for the request operation
// - cid: Client ID to send request to
// - p: Request packet to send
//
// Returns:
// - *packet.Response: Response packet if successful, nil otherwise
// - error: Error if request fails, nil otherwise
func (s *server) SendRequest(ctx context.Context, cid string, p *packet.Request) (*packet.Response, error) {
	if s.closed.Load() {
		return nil, xerr.ServerIsClosed
	}
	if c, ok := s.conns.Get(cid); ok {
		return c.SendRequest(ctx, p)
	}
	return nil, xerr.ConnectionNotFound
}

// onPacket handles incoming packets from clients.
//
// Parameters:
// - p: Packet received from client
// - c: Connection that received the packet
func (s *server) onPacket(p packet.Packet, c network.Conn) {
	if s.closed.Load() {
		return
	}
	switch p.Type() {
	case packet.MESSAGE:
		s.onMessage(c, p.(*packet.Message))
	case packet.MESSACK:
		c.RecvMessack(p.(*packet.Messack))
	case packet.REQUEST:
		s.onRequest(c, p.(*packet.Request))
	case packet.RESPONSE:
		c.RecvResponse(p.(*packet.Response))
	case packet.PING:
		c.SendPong()
	case packet.PONG:
		c.RecvPong()
	case packet.CLOSE:
		c.Close()
	default:
		break
	}
}

// onConnect handles new client connections.
//
// Parameters:
// - p: Connect packet from client
// - c: Connection that was accepted
//
// Returns:
// - packet.ConnectCode: Connection result code
func (s *server) onConnect(p *packet.Connect, c network.Conn) packet.ConnectCode {
	code := s.connectHandler(p)
	if code == packet.ConnectAccepted {
		if old, loaded := s.conns.Swap(p.Identity.ClientID, c); loaded {
			old.CloseClode(packet.CloseDuplicateLogin)
		}
	}
	return code
}

// onMessage handles incoming message packets from clients.
//
// Parameters:
// - c: Connection that received the message
// - p: Message packet received
func (s *server) onMessage(c network.Conn, p *packet.Message) {
	err := s.messageHandler(p, c.ID())
	if err != nil {
		s.logger.Error("failed to handle message", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		return
	}
	if p.Qos == packet.MessageQos1 {
		if err := c.SendPacket(context.Background(), p.Ack()); err != nil {
			s.logger.Error("failed to send messack", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		}
	}
}

// onRequest handles incoming request packets from clients.
//
// Parameters:
// - c: Connection that received the request
// - p: Request packet received
func (s *server) onRequest(c network.Conn, p *packet.Request) {
	res, err := s.requestHandler(p, c.ID())
	if err != nil {
		s.logger.Error("failed to handle request", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		res = p.Response(packet.StatusNotFound)
	}
	if err := c.SendPacket(context.Background(), res); err != nil {
		s.logger.Error("failed to send response", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
	}
}

// onClose handles connection close events.
//
// Parameters:
// - c: Connection that was closed
func (s *server) onClose(c network.Conn) {
	id := c.ID()
	if id == nil {
		return
	}
	if s.conns.Delete(id.ClientID) {
		s.closeHandler(id)
	}
}
