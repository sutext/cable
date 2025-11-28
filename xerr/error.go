package xerr

type Error uint16

const (
	RequestTimeout Error = iota
	ServerIsClosed
	ServerAlreadyClosed
	ConnectionIsClosed
	SendingQueueIsFull
	NetworkNotSupported
	ConnectionNotFound
	RequestHandlerNotFound
	BrokerPeerNotReady
	InvalidUserID
	InvalidChannel
	InvalidPeerMessageFlag
)

var errorMap = map[Error]string{
	RequestTimeout:         "request timeout",
	ServerIsClosed:         "server is closed",
	ServerAlreadyClosed:    "server already closed",
	ConnectionIsClosed:     "connection is closed",
	SendingQueueIsFull:     "sending queue is full",
	NetworkNotSupported:    "network not supported",
	ConnectionNotFound:     "connection not found",
	RequestHandlerNotFound: "request handler not found",
	BrokerPeerNotReady:     "broker peer not ready",
	InvalidUserID:          "invalid user id",
	InvalidChannel:         "invalid channel",
	InvalidPeerMessageFlag: "invalid peer message flag",
}

func (e Error) Error() string {
	return errorMap[e]
}
func (e Error) String() string {
	return errorMap[e]
}
