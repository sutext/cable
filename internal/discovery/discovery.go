package discovery

import (
	"fmt"
	"net"
	"sync"
	"time"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Discovery interface {
	Serve() error
	Request() (map[uint64]string, error)
	OnRequest(func(uint64, string))
	Shutdown() error
}

type discovery struct {
	id       uint64
	ipaddr   string
	mu       sync.Mutex
	addr     *net.UDPAddr
	conn     *net.UDPConn
	req      func(id uint64, addr string)
	logger   *xlog.Logger
	listener *net.UDPConn
	respChan chan *packet.Response
}

func New(id uint64, port uint16) Discovery {
	ip, err := getLocalIP()
	if err != nil {
		panic(err)
	}
	m := &discovery{
		id:     id,
		ipaddr: fmt.Sprintf("%s:%d", ip, port),
		logger: xlog.With("GROUP", "DISCOVERY"),
		addr:   &net.UDPAddr{IP: net.IPv4(224, 0, 0, 9), Port: 9999},
	}
	return m
}

func (m *discovery) Serve() error {
	listener, err := net.ListenMulticastUDP("udp", nil, m.addr)
	if err != nil {
		return err
	}
	m.listener = listener
	go func() {
		if err := m.listen(); err != nil {
			m.logger.Info("discovery listen stoped", xlog.Err(err))
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
		dec := coder.NewDecoder(req.Body)
		id, err := dec.ReadUInt64()
		if err != nil {
			m.logger.Error("decode request error", xlog.Err(err))
			continue
		}
		ipaddr, err := dec.ReadString()
		if err != nil {
			m.logger.Error("decode request error", xlog.Err(err))
			continue
		}
		m.req(id, ipaddr)
		ec := coder.NewEncoder()
		ec.WriteUInt64(m.id)
		ec.WriteString(m.ipaddr)
		resp := req.Response(packet.StatusOK, ec.Bytes())
		bytes, err := packet.Marshal(resp)
		if err != nil {
			m.logger.Error("marshal response error", xlog.Err(err))
		}
		if _, err = listener.WriteToUDP(bytes, addr); err != nil {
			m.logger.Error("write response error", xlog.Err(err))
		}
	}
}

func (m *discovery) Shutdown() error {
	err := m.conn.Close()
	err = m.listener.Close()
	return err
}
func (m *discovery) OnRequest(f func(uint64, string)) {
	m.req = f
}
func (m *discovery) listen() error {
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
func (m *discovery) Request() (r map[uint64]string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.respChan != nil {
		return nil, fmt.Errorf("Muticast request is already in progress")
	}
	ec := coder.NewEncoder()
	ec.WriteUInt64(m.id)
	ec.WriteString(m.ipaddr)
	req := packet.NewRequest("discovery", ec.Bytes())
	reqdata, err := packet.Marshal(req)
	if err != nil {
		return r, err
	}
	m.respChan = make(chan *packet.Response, 8)
	time.AfterFunc(time.Second*5, func() {
		close(m.respChan)
		m.respChan = nil
	})
	_, err = m.conn.WriteToUDP(reqdata, m.addr)
	if err != nil {
		return r, err
	}
	r = make(map[uint64]string)
	for resp := range m.respChan {
		dc := coder.NewDecoder(resp.Body)
		id, err := dc.ReadUInt64()
		if err != nil {
			m.logger.Error("decode request error", xlog.Err(err))
			continue
		}
		ipaddr, err := dc.ReadString()
		if err != nil {
			m.logger.Error("decode request error", xlog.Err(err))
			continue
		}
		r[id] = ipaddr
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
