package muticast

import (
	"fmt"
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
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
	req      func(string) uint32
	logger   *xlog.Logger
	respChan chan *packet.Response
}

func New(id string) *muticast {
	m := &muticast{
		id:     id,
		logger: xlog.With("GROUP", "MUTICAST"),
		addr:   &net.UDPAddr{IP: net.IPv4(224, 0, 0, 9), Port: 9999},
	}
	return m
}

func (m *muticast) Serve() error {
	listener, err := net.ListenMulticastUDP("udp4", nil, m.addr)
	if err != nil {
		return err
	}
	go func() {
		if err := m.listen(); err != nil {
			m.logger.Error("listen error", err)
		}
	}()
	defer listener.Close()
	buffer := make([]byte, 1024)
	for {
		n, addr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			m.logger.Error("unmarshal packet error", err)
			continue
		}
		if p.Type() != packet.REQUEST {
			m.logger.Errorf("packet type is not request")
			continue
		}
		req := p.(*packet.Request)
		if req.Method != "discovery" {
			m.logger.Errorf("request method is not discovery")
			continue
		}
		go func() {
			count := m.req(string(req.Content))
			enc := coder.NewEncoder()
			enc.WriteUInt32(count)
			enc.WriteString(m.id)
			resp := packet.NewResponse(req.ID, enc.Bytes())
			bytes, err := packet.Marshal(resp)
			if err != nil {
				m.logger.Error("marshal response error", err)
			}
			if _, err = listener.WriteToUDP(bytes, addr); err != nil {
				m.logger.Error("write response error", err)
			}
		}()
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
			m.logger.Error("unmarshal packet error", err)
			continue
		}
		if p.Type() != packet.RESPONSE {
			m.logger.Errorf("packet type is not response")
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
	time.AfterFunc(time.Second*5, func() {
		close(m.respChan)
		m.respChan = nil
	})
	r = make(map[string]uint32)
	for resp := range m.respChan {
		denc := coder.NewDecoder(resp.Content)
		count, err := denc.ReadUInt32()
		if err != nil {
			m.logger.Error("decode response error", err)
			continue
		}
		id, err := denc.ReadString()
		if err != nil {
			m.logger.Error("decode response error", err)
			continue
		}
		r[id] = count
	}
	return r, nil
}
