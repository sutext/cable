package client

type Status uint8

const (
	StatusUnknown Status = iota
	StatusClosed
	StatusOpened
	StatusOpening
	StatusClosing
)

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

type CloseReason uint16

const (
	CloseReasonNormal CloseReason = iota
	CloseReasonPingTimeout
	CloseReasonNetworkError
	CloseReasonServerClose
)

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
func (c CloseReason) Error() string {
	return c.String()
}
