package muticast

import (
	"fmt"
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
)

type Muticast interface {
	Serve() error
	Request() (map[string]uint32, error)
	OnRequest(func(string) uint32)
	Shutdown()
}

type muticast struct {
	id       string
	mu       sync.Mutex
	addr     *net.UDPAddr
	conn     *net.UDPConn
	result   chan string
	req      func(string) uint32
	respChan chan *packet.Response
}

func New(id string) *muticast {
	m := &muticast{
		id:   id,
		addr: &net.UDPAddr{IP: net.ParseIP("224.0.0.9"), Port: 9999},
	}
	return m
}

func (m *muticast) Serve() error {
	conn, err := net.ListenMulticastUDP("udp4", nil, m.addr)
	if err != nil {
		return err
	}
	go m.listen()
	defer conn.Close()
	buffer := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println(err)
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			continue
		}
		if p.Type() != packet.REQUEST {
			continue
		}
		req := p.(*packet.Request)
		if req.Method != "discovery" {
			continue
		}
		count := m.req(string(req.Content))
		enc := coder.NewEncoder()
		enc.WriteUInt32(count)
		enc.WriteString(m.id)
		resp := packet.NewResponse(req.ID, enc.Bytes())
		bytes, err := packet.Marshal(resp)
		if err != nil {
			return err
		}
		_, err = conn.WriteToUDP(bytes, addr)
		if err != nil {
			return err
		}
	}
}
func (m *muticast) Shutdown() {
	m.conn.Close()
}
func (m *muticast) OnRequest(f func(string) uint32) {
	m.req = f
}
func (m *muticast) listen() error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	m.conn = conn
	defer conn.Close()
	buffer := make([]byte, 1024)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			continue
		}
		if p.Type() != packet.RESPONSE {
			continue
		}
		m.respChan <- p.(*packet.Response)
	}
}
func (m *muticast) Request() (r map[string]uint32, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.respChan != nil {
		return nil, fmt.Errorf("Muticast request is already in progress")
	}
	req := packet.NewRequest("discovery", []byte(m.id))
	reqdata, err := packet.Marshal(req)
	if err != nil {
		return r, err
	}
	_, err = m.conn.WriteToUDP(reqdata, m.addr)
	if err != nil {
		return r, err
	}
	m.respChan = make(chan *packet.Response)
	r = make(map[string]uint32)
	time.AfterFunc(time.Second*5, func() {
		m.mu.Lock()
		close(m.respChan)
		m.respChan = nil
		m.mu.Unlock()
	})
	for resp := range m.respChan {
		denc := coder.NewDecoder(resp.Content)
		count, err := denc.ReadUInt32()
		if err != nil {
			continue
		}
		id, err := denc.ReadString()
		if err != nil {
			continue
		}
		r[id] = count
	}
	return r, nil
}
