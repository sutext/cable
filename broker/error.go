package broker

type Error uint8

const (
	ErrInvalidChannel Error = iota
	ErrInvalidUserID
	ErrInvalidMessageFlag
	ErrRequestHandlerNotFound
)

var errorMap = map[Error]string{
	ErrInvalidChannel:         "Invalid channel",
	ErrInvalidUserID:          "Invalid user ID",
	ErrInvalidMessageFlag:     "Invalid message flag",
	ErrRequestHandlerNotFound: "Request handler not found",
}

func (e Error) Error() string {
	return errorMap[e]
}
func (e Error) String() string {
	return errorMap[e]
}
