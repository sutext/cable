// Package client provides a client implementation for the cable protocol.
// It supports multiple network protocols (TCP, UDP, WebSocket, QUIC) and provides
// features like automatic reconnection, heartbeat detection, and request-response mechanism.
package client

// Status represents the current state of the client connection.
type Status uint8

// Client connection status constants.
const (
	// StatusUnknown indicates the client status is unknown.
	StatusUnknown Status = iota
	// StatusClosed indicates the client connection is closed.
	StatusClosed
	// StatusOpened indicates the client connection is open and ready.
	StatusOpened
	// StatusOpening indicates the client is in the process of opening a connection.
	StatusOpening
	// StatusClosing indicates the client is in the process of closing a connection.
	StatusClosing
)

// String returns the string representation of the Status.
func (s Status) String() string {
	switch s {
	case StatusUnknown:
		return "Unknown"
	case StatusClosed:
		return "Closed"
	case StatusOpened:
		return "Opened"
	case StatusOpening:
		return "Opening"
	case StatusClosing:
		return "Closing"
	default:
		return "Unknown"
	}
}

// CloseReason represents the reason for closing a connection.
type CloseReason uint16

// Connection close reason constants.
const (
	// CloseReasonNormal indicates the connection was closed normally.
	CloseReasonNormal CloseReason = iota
	// CloseReasonPingTimeout indicates the connection was closed due to ping timeout.
	CloseReasonPingTimeout
	// CloseReasonNetworkError indicates the connection was closed due to network error.
	CloseReasonNetworkError
	// CloseReasonServerClose indicates the connection was closed by the server.
	CloseReasonServerClose
)

// String returns the string representation of the CloseReason.
func (c CloseReason) String() string {
	switch c {
	case CloseReasonNormal:
		return "Normal"
	case CloseReasonPingTimeout:
		return "Ping Timeout"
	case CloseReasonNetworkError:
		return "Network Error"
	case CloseReasonServerClose:
		return "Server Close"
	default:
		return "Unknown"
	}
}

// Error implements the error interface for CloseReason.
func (c CloseReason) Error() string {
	return c.String()
}
