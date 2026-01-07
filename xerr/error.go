package xerr

type Error uint16

const (
	PeerNotReady Error = iota
	InvalidUserID
	InvalidChannel
	RequestTimeout
	MessageTimeout
	ServerIsClosed
	RaftNodeNotReady
	InvalidPeerMessage
	ConnectionIsClosed
	SendingQueueIsFull
	ConnectionNotFound
	ServerAlreadyClosed
	TransportNotSupported
	RequestHandlerNotFound
	InvalidPeerMessageFlag
)

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
	TransportNotSupported:  "transport not supported",
	InvalidPeerMessageFlag: "invalid peer message flag",
	RequestHandlerNotFound: "request handler not found",
}

func (e Error) Error() string {
	return errorMap[e]
}
func (e Error) String() string {
	return errorMap[e]
}
