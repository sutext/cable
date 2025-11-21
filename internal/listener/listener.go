package listener

type Listener interface {
	Listen() error
	Accept() (Conn, error)
}
