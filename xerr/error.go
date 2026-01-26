// Package xerr provides a unified error handling mechanism for the cable project.
// It defines a set of standard error codes with descriptive messages for consistent error reporting.
package xerr

// Error defines a unified error type for the cable project, represented as a uint16.
type Error uint16

// Error constants for the cable project.
const (
	PeerNotReady           Error = iota // Broker peer is not ready to accept requests
	InvalidUserID                       // Invalid user ID provided
	InvalidChannel                      // Invalid channel provided
	RequestTimeout                      // Request timed out
	MessageTimeout                      // Message delivery timed out
	ServerIsClosed                      // Server is closed
	RaftNodeNotReady                    // Raft node is not ready
	InvalidPeerMessage                  // Invalid peer message received
	ConnectionIsClosed                  // Connection is closed
	SendingQueueIsFull                  // Sending queue is full
	ConnectionNotFound                  // Connection not found
	ServerAlreadyClosed                 // Server is already closed
	NetworkNotSupported                 // Network type not supported
	InvalidPeerMessageFlag              // Invalid peer message flag
)

// errorMap maps Error codes to their descriptive messages.
var errorMap = map[Error]string{
	PeerNotReady:           "broker peer not ready",
	InvalidUserID:          "invalid user id",
	RequestTimeout:         "request timeout",
	MessageTimeout:         "message timeout",
	ServerIsClosed:         "server is closed",
	InvalidChannel:         "invalid channel",
	RaftNodeNotReady:       "raft node not ready",
	InvalidPeerMessage:     "invalid peer message",
	ConnectionIsClosed:     "connection is closed",
	SendingQueueIsFull:     "sending queue is full",
	ConnectionNotFound:     "connection not found",
	ServerAlreadyClosed:    "server already closed",
	NetworkNotSupported:    "network not supported",
	InvalidPeerMessageFlag: "invalid peer message flag",
}

// Error implements the error interface for Error type.
// Returns the descriptive message for the error code.
func (e Error) Error() string {
	return errorMap[e]
}

// String returns the string representation of the Error.
// Returns the descriptive message for the error code.
func (e Error) String() string {
	return errorMap[e]
}
