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
	Request() (map[string]int32, error)
	OnRequest(func(string) int32)
	Shutdown() error
}

type muticast struct {
	idaddr   string
	mu       sync.Mutex
	addr     *net.UDPAddr
	conn     *net.UDPConn
	req      func(idip string) int32
	logger   *xlog.Logger
	listener *net.UDPConn
	respChan chan *packet.Response
}

func New(id, port string) *muticast {
	ip, err := getLocalIP()
	if err != nil {
		panic(err)
	}
	m := &muticast{
		idaddr: fmt.Sprintf("%s@%s:%s", id, ip, port),
		logger: xlog.With("GROUP", "MUTICAST"),
		addr:   &net.UDPAddr{IP: net.IPv4(224, 0, 0, 9), Port: 9999},
	}
	return m
}

func (m *muticast) Serve() error {
	listener, err := net.ListenMulticastUDP("udp", nil, m.addr)
	if err != nil {
		return err
	}
	m.listener = listener
	go func() {
		if err := m.listen(); err != nil {
			m.logger.Error("listen error", xlog.Err(err))
		}
	}()
	defer listener.Close()
	buffer := make([]byte, packet.MAX_UDP)
	for {
		n, addr, err := listener.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			m.logger.Error("unmarshal packet error", xlog.Err(err))
			continue
		}
		if p.Type() != packet.REQUEST {
			m.logger.Error("packet type is not request")
			continue
		}
		req := p.(*packet.Request)
		if req.Method != "discovery" {
			m.logger.Error("request method is not discovery")
			continue
		}
		count := m.req(string(req.Content))
		enc := coder.NewEncoder()
		enc.WriteInt32(count)
		enc.WriteString(m.idaddr)
		resp := packet.NewResponse(req.ID, enc.Bytes())
		bytes, err := packet.Marshal(resp)
		if err != nil {
			m.logger.Error("marshal response error", xlog.Err(err))
		}
		if _, err = listener.WriteToUDP(bytes, addr); err != nil {
			m.logger.Error("write response error", xlog.Err(err))
		}
	}
}

func (m *muticast) Shutdown() error {
	err := m.conn.Close()
	err = m.listener.Close()
	return err
}
func (m *muticast) OnRequest(f func(string) int32) {
	m.req = f
}
func (m *muticast) listen() error {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	m.conn = conn
	defer conn.Close()
	buffer := make([]byte, packet.MAX_UDP)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			m.logger.Error("unmarshal packet error", xlog.Err(err))
			continue
		}
		if p.Type() != packet.RESPONSE {
			m.logger.Error("packet type is not response")
			continue
		}
		m.respChan <- p.(*packet.Response)
	}
}
func (m *muticast) Request() (r map[string]int32, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.respChan != nil {
		return nil, fmt.Errorf("Muticast request is already in progress")
	}
	req := packet.NewRequest("discovery", []byte(m.idaddr))
	reqdata, err := packet.Marshal(req)
	if err != nil {
		return r, err
	}
	m.respChan = make(chan *packet.Response, 128)
	time.AfterFunc(time.Second*6, func() {
		close(m.respChan)
		m.respChan = nil
	})
	_, err = m.conn.WriteToUDP(reqdata, m.addr)
	if err != nil {
		return r, err
	}
	r = make(map[string]int32)
	for resp := range m.respChan {
		denc := coder.NewDecoder(resp.Content)
		count, err := denc.ReadInt32()
		if err != nil {
			m.logger.Error("decode response error", xlog.Err(err))
			continue
		}
		id, err := denc.ReadString()
		if err != nil {
			m.logger.Error("decode response error", xlog.Err(err))
			continue
		}
		r[id] = count
	}
	return r, nil
}

// getLocalIP retrieves the non-loopback local IPv4 address of the machine.
func getLocalIP() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		// Check if the interface is up and not a loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("no network interface found")
}
