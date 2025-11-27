package server

type Error uint8

const (
	ErrRequestTimeout Error = iota
	ErrServerIsClosed
	ErrServerAlreadyClosed
	ErrConnectionClosed
	ErrSendingQueueFull
	ErrNetworkNotSupport
	ErrConnectionNotFound
)

func (e Error) Error() string {
	switch e {
	case ErrRequestTimeout:
		return "Request Timeout"
	case ErrServerIsClosed:
		return "Server Is Closed"
	case ErrServerAlreadyClosed:
		return "Server Already Closed"
	case ErrConnectionClosed:
		return "Connection Closed"
	case ErrSendingQueueFull:
		return "Sending Queue Full"
	case ErrNetworkNotSupport:
		return "Network Not Support"
	case ErrConnectionNotFound:
		return "Connection Not Found"
	default:
		return "Unknown Error"
	}
}
