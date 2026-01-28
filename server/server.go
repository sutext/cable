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
	"sutext.github.io/cable/stats"
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
	// Shutdown shuts down the server and closes all connections.
	Shutdown(ctx context.Context) error
	/// IsActive returns true if the client with the given CID is active.
	IsActive(cid string) bool
	// KickConn kicks a client out of the server by CID.
	KickConn(cid string) bool
	// ConnInfo returns the connection information of a client by CID.
	ConnInfo(cid string) (*ConnInfo, bool)
	// ConnCount returns the total number of active connections.
	ConnCount() int32
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
	queueCapacity  int32                           // Maximum sending queue capacity
	statsHandler   stats.Handler                   // Handler for statistics events
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
		statsHandler:   options.statsHandler,
		queueCapacity:  options.queueCapacity,
		closeHandler:   options.closeHandler,
		connectHandler: options.connectHandler,
		messageHandler: options.messageHandler,
		requestHandler: options.requestHandler,
	}
	// Create transport based on network protocol
	switch s.network {
	case NetworkWS:
		s.transport = network.NewWS(s)
	case NetworkWSS:
		s.transport = network.NewWSS(options.tlsConfig, s)
	case NetworkTCP:
		s.transport = network.NewTCP(s)
	case NetworkTLS:
		s.transport = network.NewTLS(options.tlsConfig, s)
	case NetworkUDP:
		s.transport = network.NewUDP(s)
	case NetworkQUIC:
		s.transport = network.NewQUIC(options.quicConfig, s)
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

// KickConn kicks a client out of the server by CID.
//
// Parameters:
// - cid: Client ID to kick
//
// Returns:
// - bool: True if the client was kicked, false if not found
func (s *server) KickConn(cid string) bool {
	if c, ok := s.conns.Get(cid); ok {
		c.CloseCode(packet.CloseKickedOut)
		return true
	}
	return false
}

// ConnCount returns the total number of active connections.
//
// Returns:
// - int32: Number of active connections
func (s *server) ConnCount() int32 {
	return s.conns.Len()
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

// ExpelAllConns expels all connections from the server.
func (s *server) ExpelAllConns() {
	s.conns.Range(func(key string, c network.Conn) bool {
		c.CloseCode(packet.CloseServerExpeled)
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
	var err error
	if s.statsHandler != nil {
		beginTime := time.Now()
		ctx = s.statsHandler.MessageBegin(ctx, &stats.MessageBegin{
			Kind:        p.Kind,
			Network:     s.network,
			BeginTime:   beginTime,
			IsIncoming:  false,
			PayloadSize: len(p.Payload),
		})
		defer s.statsHandler.MessageEnd(ctx, &stats.MessageEnd{
			Error:      err,
			EndTime:    time.Now(),
			Network:    s.network,
			BeginTime:  beginTime,
			IsIncoming: false,
		})
	}
	if s.closed.Load() {
		err = xerr.ServerIsClosed
		return err
	}
	c, ok := s.conns.Get(cid)
	if !ok {
		err = xerr.ConnectionNotFound
		return err
	}
	err = c.SendMessage(ctx, p)
	return err
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
	var err error
	var res *packet.Response
	if s.statsHandler != nil {
		beginTime := time.Now()
		ctx = s.statsHandler.RequestBegin(ctx, &stats.RequestBegin{
			Method:     p.Method,
			Network:    s.network,
			BodySize:   len(p.Body),
			BeginTime:  beginTime,
			IsIncoming: false,
		})
		defer func() {
			end := &stats.RequestEnd{
				Error:      err,
				EndTime:    time.Now(),
				Network:    s.network,
				BeginTime:  beginTime,
				IsIncoming: false,
			}
			if res != nil {
				end.StatusCode = res.Code
				end.BodySize = len(res.Body)
			}
			s.statsHandler.RequestEnd(ctx, end)
		}()
	}
	if s.closed.Load() {
		err = xerr.ServerIsClosed
		return nil, err
	}
	c, ok := s.conns.Get(cid)
	if !ok {
		err = xerr.ConnectionNotFound
		return nil, err
	}
	res, err = c.SendRequest(ctx, p)
	return res, err
}
func (s *server) Logger() *xlog.Logger {
	return s.logger
}
func (s *server) GetConn(cid string) (network.Conn, bool) {
	return s.conns.Get(cid)
}
func (s *server) QueueCapacity() int32 {
	return s.queueCapacity
}

// OnConnect handles new client connections.
//
// Parameters:
// - p: Connect packet from client
// - c: Connection that was accepted
func (s *server) OnConnect(c network.Conn, p *packet.Connect) error {
	ctx := context.Background()
	var err error
	var code packet.ConnectCode
	if s.statsHandler != nil {
		beginTime := time.Now()
		ctx = s.statsHandler.ConnectBegin(ctx, &stats.ConnBegin{
			BeginTime: beginTime,
		})
		defer s.statsHandler.ConnectEnd(ctx, &stats.ConnEnd{
			BeginTime: beginTime,
			EndTime:   time.Now(),
			Error:     err,
			Code:      code,
		})
	}
	code = s.connectHandler(ctx, p)
	if code != packet.ConnectAccepted {
		c.ConnackCode(code)
		c.Close()
		err = code
		s.logger.Error("failed to handle connect", xlog.Err(err), xlog.Uid(p.Identity.UserID), xlog.Cid(p.Identity.ClientID))
		return err
	}
	if old, loaded := s.conns.Swap(p.Identity.ClientID, c); loaded {
		old.CloseCode(packet.CloseDuplicateLogin)
	}
	c.ConnackCode(packet.ConnectAccepted)
	return nil
}

// OnPacket handles incoming packets from clients.
//
// Parameters:
// - p: Packet received from client
// - c: Connection that received the packet
func (s *server) OnPacket(c network.Conn, p packet.Packet) {
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

// OnClose handles connection close events.
//
// Parameters:
// - c: Connection that was closed
func (s *server) OnClose(c network.Conn) {
	id := c.ID()
	if id == nil {
		return
	}
	if s.conns.Delete(id.ClientID) {
		s.closeHandler(id)
	}
}

// onMessage handles incoming message packets from clients.
//
// Parameters:
// - c: Connection that received the message
// - p: Message packet received
func (s *server) onMessage(c network.Conn, p *packet.Message) {
	ctx := context.Background()
	var err error
	if s.statsHandler != nil {
		beginTime := time.Now()
		ctx = s.statsHandler.MessageBegin(ctx, &stats.MessageBegin{
			Kind:        p.Kind,
			Network:     s.network,
			BeginTime:   beginTime,
			IsIncoming:  true,
			PayloadSize: len(p.Payload),
		})
		defer s.statsHandler.MessageEnd(ctx, &stats.MessageEnd{
			Kind:       p.Kind,
			Error:      err,
			Network:    s.network,
			EndTime:    time.Now(),
			BeginTime:  beginTime,
			IsIncoming: true,
		})
	}
	err = s.messageHandler(ctx, p, c.ID())
	if err != nil {
		s.logger.Error("failed to handle message", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		return
	}
	if p.Qos == packet.MessageQos1 {
		if err = c.SendPacket(ctx, p.Ack()); err != nil {
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
	ctx := context.Background()
	var err error
	var res *packet.Response
	if s.statsHandler != nil {
		beginTime := time.Now()
		ctx = s.statsHandler.RequestBegin(ctx, &stats.RequestBegin{
			Method:     p.Method,
			Network:    s.network,
			BodySize:   len(p.Body),
			BeginTime:  beginTime,
			IsIncoming: true,
		})
		defer func() {
			s.statsHandler.RequestEnd(ctx, &stats.RequestEnd{
				Error:      err,
				EndTime:    time.Now(),
				Method:     p.Method,
				Network:    s.network,
				BodySize:   len(res.Body),
				BeginTime:  beginTime,
				StatusCode: res.Code,
				IsIncoming: true,
			})
		}()

	}
	body, rerr := s.requestHandler(ctx, p, c.ID())
	if rerr != nil {
		err = rerr
		s.logger.Error("failed to handle request", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
		code, ok := rerr.(packet.StatusCode)
		if !ok {
			code = packet.StatusInternalError
		}
		res = p.Response(code)
	} else {
		res = p.Response(packet.StatusOK, body)
	}
	if err = c.SendPacket(ctx, res); err != nil {
		s.logger.Error("failed to send response", xlog.Err(err), xlog.Uid(c.ID().UserID), xlog.Cid(c.ID().ClientID))
	}
}
