package discovery

type Endpoint struct {
	ID   string
	IP   string
	Port string
}

func (e *Endpoint) String() string {
	return e.ID + "@" + e.IP + ":" + e.Port
}

func (e *Endpoint) Address() string {
	return e.IP + ":" + e.Port
}
