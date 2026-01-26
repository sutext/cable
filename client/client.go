// Package client provides a client implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like automatic reconnection, heartbeat detection, and request-response mechanism.
package client

import (
	"context"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sutext.github.io/cable/internal/keepalive"
	"sutext.github.io/cable/internal/queue"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

// Error defines client-specific error codes.
type Error uint8

// Error constants for the client package.
const (
	ErrInvalidPacket      Error = 0 // Invalid packet received
	ErrRequestTimeout     Error = 1 // Request timed out
	ErrConnectionFailed   Error = 2 // Connection failed
	ErrConnectionClosed   Error = 3 // Connection closed
	ErrConnectionNotReady Error = 4 // Connection not ready
)

// Error implements the error interface for Error type.
func (e Error) Error() string {
	switch e {
	case ErrInvalidPacket:
		return "invalid packet"
	case ErrRequestTimeout:
		return "request timeout"
	case ErrConnectionFailed:
		return "connection failed"
	case ErrConnectionClosed:
		return "connection closed"
	case ErrConnectionNotReady:
		return "connection not ready"
	default:
		return "unknown error"
	}
}

// Client defines the interface for a cable client.
// It provides methods for connection management, message sending, and request handling.
type Client interface {
	// ID returns the client's identity.
	ID() *packet.Identity
	// Status returns the current client status.
	Status() Status
	// Connect establishes a connection with the given identity.
	Connect()
	// IsReady returns whether the client is ready to send messages.
	IsReady() bool
	// SendMessage sends a message to the server.
	SendMessage(ctx context.Context, p *packet.Message) error
	// SendRequest sends a request to the server and returns the response.
	SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error)
}

// client implements the Client interface.
type client struct {
	id             *packet.Identity                 // Client identity
	conn           Conn                             // Underlying connection
	closed         atomic.Bool                      // Whether the client is closed
	status         Status                           // Current client status
	logger         *xlog.Logger                     // Logger instance
	retrier        *Retrier                         // Retry mechanism for reconnections
	handler        Handler                          // Packet handler
	retrying       bool                             // Whether the client is retrying a connection
	pingChan       chan struct{}                    // Channel for ping/pong communication
	pingLock       sync.Mutex                       // Lock for ping operations
	sendQueue      *queue.Queue                     // Queue for outgoing packets
	keepalive      *keepalive.KeepAlive             // Keepalive mechanism
	messageID      atomic.Uint64                    // Message ID generator
	requestID      atomic.Uint64                    // Request ID generator
	statusLock     sync.RWMutex                     // Lock for status changes
	pingTimeout    time.Duration                    // Timeout for ping operations
	pingInterval   time.Duration                    // Interval for sending pings
	requestLock    sync.Mutex                       // Lock for request tasks
	messageLock    sync.Mutex                       // Lock for message tasks
	requestTasks   map[uint16]chan *packet.Response // Pending request tasks
	messageTasks   map[uint16]chan *packet.Messack  // Pending message tasks
	writeTimeout   time.Duration                    // Timeout for write operations
	requestTimeout time.Duration                    // Timeout for requests
	messageTimeout time.Duration                    // Timeout for messages
}

// New creates a new Client instance with the specified address and options.
// Supports multiple network protocols: TCP, UDP, WebSocket, QUIC.
func New(endpoint string, options ...Option) Client {
	strs := strings.Split(endpoint, "://")
	if len(strs) != 2 {
		panic("invalid endpoint")
	}
	network := strs[0]
	address := strs[1]
	if network == NetworkWS || network == NetworkWSS {
		address = endpoint
	}
	opts := newOptions(options...)
	var conn Conn
	switch network {
	case NetworkWS:
		conn = newWSConn(address)
	case NetworkWSS:
		conn = newWSSConn(address, opts.tlsConfig)
	case NetworkTCP:
		conn = newTCPConn(address)
	case NetworkTLS:
		conn = newTLSConn(address, opts.tlsConfig)
	case NetworkUDP:
		conn = newUDPConn(address, opts.id.ClientID)
	case NetworkQUIC:
		conn = newQUICConn(address, opts.quicConfig)
	default:
		panic("unknown network")
	}
	c := &client{
		id:             opts.id,
		conn:           conn,
		status:         StatusUnknown,
		logger:         opts.logger,
		retrier:        opts.retrier,
		handler:        opts.handler,
		sendQueue:      queue.New(opts.sendQueueCapacity),
		pingTimeout:    opts.pingTimeout,
		pingInterval:   opts.pingInterval,
		writeTimeout:   opts.writeTimeout,
		requestTasks:   make(map[uint16]chan *packet.Response),
		messageTasks:   make(map[uint16]chan *packet.Messack),
		requestTimeout: opts.requestTimeout,
		messageTimeout: opts.messageTimeout,
	}
	c.keepalive = keepalive.New(c.pingInterval, c)
	return c
}

// ID returns the client's identity.
func (c *client) ID() *packet.Identity {
	return c.id
}

// Close closes the client connection and releases resources.
func (c *client) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.keepalive.Stop()
		c.sendQueue.Close()
		c.conn.Close()
	}
}

// Connect establishes a connection with the server using the provided identity.
func (c *client) Connect() {
	switch c.Status() {
	case StatusOpened, StatusOpening:
		return
	}
	c._setStatus(StatusOpening)
	if err := c.reconnect(); err != nil {
		c.logger.Error("connect error", xlog.Err(err))
		c.retrying = false
		c.retrier.Reset()
		c.tryClose(err)
	}
}

// IsReady returns whether the client is ready to send messages.
func (c *client) IsReady() bool {
	return c.Status() == StatusOpened
}

// tryClose attempts to close the client connection with retry logic.
func (c *client) tryClose(err error) {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()
	if c.retrying {
		return
	}
	if c.id == nil {
		return
	}
	c.logger.Error("try close", xlog.Err(err))
	if c.status == StatusClosed || c.status == StatusClosing {
		return
	}
	if code, ok := err.(CloseReason); ok {
		if code == CloseReasonNormal {
			c._setStatus(StatusClosed)
			return
		}
	}
	if c.retrier == nil {
		c._setStatus(StatusClosed)
		return
	}
	delay, ok := c.retrier.Retry(err)
	if !ok {
		c._setStatus(StatusClosed)
		return
	}
	c.retrying = true
	c._setStatus(StatusOpening)
	time.AfterFunc(delay, func() {
		if c.Status() != StatusOpening {
			return
		}
		if err := c.reconnect(); err != nil {
			c.retrying = false
			c.tryClose(err)
		}
	})
}

// reconnect handles reconnection logic when the connection is lost.
func (c *client) reconnect() error {
	err := c.conn.Dail()
	if err != nil {
		return err
	}
	err = c.conn.WritePacket(packet.NewConnect(c.id))
	if err != nil {
		return err
	}
	p, err := c.conn.ReadPacket()
	if err != nil {
		return err
	}
	if p.Type() != packet.CONNACK {
		return ErrConnectionFailed
	}
	ack := p.(*packet.Connack)
	if ack.Code != packet.ConnectAccepted {
		return ack.Code
	}
	c.setStatus(StatusOpened)
	return nil
}

// SendPing sends a ping message to the server.
func (c *client) SendPing() {
	ctx, cancel := context.WithTimeout(context.Background(), c.pingTimeout)
	defer cancel()
	err := c.sendPing(ctx)
	if err != nil {
		c.logger.Error("send ping error", xlog.Err(err))
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			c.tryClose(CloseReasonPingTimeout)
		}
	}
}

// sendPing sends a ping message with context and waits for pong response.
func (c *client) sendPing(ctx context.Context) error {
	if !c.IsReady() {
		return ErrConnectionNotReady
	}
	c.pingLock.Lock()
	c.pingChan = make(chan struct{})
	c.pingLock.Unlock()
	defer func() {
		c.pingLock.Lock()
		close(c.pingChan)
		c.pingChan = nil
		c.pingLock.Unlock()
	}()
	if err := c.sendPacket(ctx, false, packet.NewPing()); err != nil {
		return err
	}
	select {
	case <-c.pingChan:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// sendPong sends a pong message in response to a ping.
func (c *client) sendPong() error {
	return c.sendPacket(context.Background(), true, packet.NewPong())
}

// SendMessage sends a message to the server with optional QoS.
func (c *client) SendMessage(ctx context.Context, p *packet.Message) error {
	if !c.IsReady() {
		return ErrConnectionNotReady
	}
	if p.Qos == packet.MessageQos0 {
		return c.sendPacket(ctx, false, p)
	}
	if p.ID == 0 {
		p.ID = uint16(c.messageID.Add(1) / math.MaxUint16)
	}
	return c.retryInflightMessage(ctx, p, 0)
}

// retryInflightMessage retries sending a message with QoS > 0 until acknowledged or timed out.
func (c *client) retryInflightMessage(ctx context.Context, p *packet.Message, attempts int) error {
	if attempts > 5 {
		return ErrRequestTimeout
	}
	if attempts > 0 {
		p.Dup = true
		c.logger.Warn("retry inflight message", xlog.U16("id", p.ID), xlog.Int("attempts", attempts))
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	_, err := c.sendInflightMessage(ctx, p)
	if err != nil {
		if t, ok := err.(interface{ Timeout() bool }); ok && t.Timeout() {
			p.Dup = true
			return c.retryInflightMessage(context.Background(), p, attempts+1)
		}
		return err
	}
	return nil
}

// sendInflightMessage sends a message and waits for acknowledgment.
func (c *client) sendInflightMessage(ctx context.Context, p *packet.Message) (*packet.Messack, error) {
	if !c.IsReady() {
		return nil, ErrConnectionNotReady
	}
	c.messageLock.Lock()
	ackCh := make(chan *packet.Messack)
	c.messageTasks[p.ID] = ackCh
	c.messageLock.Unlock()
	defer func() {
		c.messageLock.Lock()
		delete(c.messageTasks, p.ID)
		close(ackCh)
		c.messageLock.Unlock()
	}()
	if err := c.sendPacket(ctx, false, p); err != nil {
		return nil, err
	}
	select {
	case res := <-ackCh:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendRequest sends a request to the server and waits for the response.
func (c *client) SendRequest(ctx context.Context, p *packet.Request) (*packet.Response, error) {
	if !c.IsReady() {
		return nil, ErrConnectionNotReady
	}
	if p.ID == 0 {
		p.ID = uint16(c.requestID.Add(1) / math.MaxUint16)
	}
	c.requestLock.Lock()
	resp := make(chan *packet.Response)
	c.requestTasks[p.ID] = resp
	c.requestLock.Unlock()
	defer func() {
		c.requestLock.Lock()
		delete(c.requestTasks, p.ID)
		close(resp)
		c.requestLock.Unlock()
	}()
	if err := c.sendPacket(ctx, false, p); err != nil {
		return nil, err
	}
	select {
	case res := <-resp:
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// sendPacket sends a packet through the connection with optional priority.
func (c *client) sendPacket(ctx context.Context, jump bool, p packet.Packet) error {
	if !c.IsReady() {
		return ErrConnectionNotReady
	}
	return c.sendQueue.Push(ctx, jump, func() {
		if !c.IsReady() {
			return
		}
		err := c.conn.WritePacket(p)
		if err != nil {
			c.tryClose(err)
			return
		}
		c.keepalive.UpdateTime()
	})
}

// Status returns the current client status.
func (c *client) Status() Status {
	c.statusLock.RLock()
	defer c.statusLock.RUnlock()
	return c.status
}

// setStatus sets the client status with thread safety.
func (c *client) setStatus(status Status) {
	c.statusLock.Lock()
	defer c.statusLock.Unlock()
	c._setStatus(status)
}

// _setStatus sets the client status without locking (internal use only).
func (c *client) _setStatus(status Status) {
	if c.status == status {
		return
	}
	c.logger.Info("client status change", xlog.Str("from", c.status.String()), xlog.Str("to", status.String()))
	c.status = status
	switch status {
	case StatusClosed:
		c.conn.Close()
		c.keepalive.Stop()
	case StatusOpening, StatusClosing:
		c.keepalive.Stop()
	case StatusOpened:
		c.keepalive.Start()
		go c.recv()
	}
	go c.handler.OnStatus(status)
}

// recv continuously receives packets from the connection.
func (c *client) recv() {
	for {
		p, err := c.conn.ReadPacket()
		if err != nil {
			c.tryClose(err)
			return
		}
		go c.handlePacket(p)
	}
}

// handlePacket processes incoming packets based on their type.
func (c *client) handlePacket(p packet.Packet) {
	c.keepalive.UpdateTime()
	switch p.Type() {
	case packet.MESSAGE:
		msg := p.(*packet.Message)
		err := c.handler.OnMessage(msg)
		if err != nil {
			c.logger.Error("message handler error", xlog.Err(err))
			return
		}
		if msg.Qos == packet.MessageQos1 {
			if err := c.sendPacket(context.Background(), true, msg.Ack()); err != nil {
				c.logger.Error("send messack packet error", xlog.Err(err))
			}
		}
	case packet.MESSACK:
		p := p.(*packet.Messack)
		c.messageLock.Lock()
		ch, ok := c.messageTasks[p.ID()]
		if ok {
			ch <- p
		}
		c.messageLock.Unlock()
		if !ok {
			c.logger.Error("response task not found", xlog.U16("id", p.ID()))
		}
	case packet.REQUEST:
		p := p.(*packet.Request)
		var res *packet.Response
		body, err := c.handler.OnRequest(p)
		if err != nil {
			c.logger.Error("request handler error", xlog.Err(err))
			code, ok := err.(packet.StatusCode)
			if !ok {
				code = packet.StatusBadRequest
			}
			res = p.Response(code)
		} else {
			res = p.Response(packet.StatusOK, body)
		}
		if err := c.sendPacket(context.Background(), false, res); err != nil {
			c.logger.Error("send response packet error", xlog.Err(err))
		}
	case packet.RESPONSE:
		p := p.(*packet.Response)
		c.requestLock.Lock()
		ch, ok := c.requestTasks[p.ID()]
		if ok {
			ch <- p
		}
		c.requestLock.Unlock()
		if !ok {
			c.logger.Error("response task not found", xlog.U16("id", p.ID()))
		}
	case packet.PING:
		if err := c.sendPong(); err != nil {
			c.logger.Error("send pong error", xlog.Err(err))
		}
	case packet.PONG:
		c.pingLock.Lock()
		if c.pingChan != nil {
			c.pingChan <- struct{}{}
		}
		c.pingLock.Unlock()
	case packet.CLOSE:
		c.tryClose(p.(*packet.Close).Code)
	default:

	}
}
