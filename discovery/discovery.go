package discovery

import (
	"fmt"
	"net"
	"time"

	"sutext.github.io/cable/coder"
	"sutext.github.io/cable/packet"
	"sutext.github.io/cable/xlog"
)

type Handler interface {
	OnNodeJoin(nodeId uint64, nodeAddr string)
}

type Discovery interface {
	Start()
	Shutdown() error
	SetHandler(handler Handler)
}
type discovery struct {
	id       uint64
	conn     *net.UDPConn
	ipaddr   string
	logger   *xlog.Logger
	handler  Handler
	address  *net.UDPAddr
	listener *net.UDPConn
	stopChan chan struct{}
}

func NewMuticast(id uint64, port uint16, logger *xlog.Logger) Discovery {
	ip, err := getLocalIP()
	if err != nil {
		panic(err)
	}
	m := &discovery{
		id:       id,
		ipaddr:   fmt.Sprintf("%s:%d", ip, port),
		logger:   logger,
		address:  &net.UDPAddr{IP: net.IPv4(224, 0, 0, 9), Port: 9999},
		stopChan: make(chan struct{}),
	}
	return m
}
func (m *discovery) SetHandler(handler Handler) {
	m.handler = handler
}
func (m *discovery) Shutdown() error {
	err := m.conn.Close()
	err = m.listener.Close()
	close(m.stopChan)
	return err
}
func (m *discovery) Start() {
	go func() {
		if err := m.listen(); err != nil {
			m.logger.Info("discovery listener stoped", xlog.Err(err))
		}
	}()
	go func() {
		if err := m.watch(); err != nil {
			m.logger.Info("discovery watch stoped", xlog.Err(err))
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Second * 5)
		defer ticker.Stop()
		for {
			select {
			case <-m.stopChan:
				return
			case <-ticker.C:
				if err := m.heartbeat(); err != nil {
					m.logger.Error("discovery heartbeat error", xlog.Err(err))
				}
			}
		}
	}()
}

func (m *discovery) listen() error {
	listener, err := net.ListenMulticastUDP("udp", nil, m.address)
	if err != nil {
		return err
	}
	m.listener = listener
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
		m.handler.OnNodeJoin(id, ipaddr)
		ec := coder.NewEncoder()
		ec.WriteUInt64(m.id)
		ec.WriteString(m.ipaddr)
		resp := req.Response(packet.StatusOK, ec.Pick())
		bytes, err := packet.Marshal(resp)
		if err != nil {
			m.logger.Error("marshal response error", xlog.Err(err))
		}
		if _, err = listener.WriteToUDP(bytes, addr); err != nil {
			m.logger.Error("write response error", xlog.Err(err))
		}
	}
}

func (m *discovery) watch() error {
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
		var resp = p.(*packet.Response)
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
		m.handler.OnNodeJoin(id, ipaddr)
	}
}
func (m *discovery) heartbeat() error {
	ec := coder.NewEncoder()
	ec.WriteUInt64(m.id)
	ec.WriteString(m.ipaddr)
	req := packet.NewRequest("discovery", ec.Pick())
	reqdata, err := packet.Marshal(req)
	if err != nil {
		return err
	}
	_, err = m.conn.WriteToUDP(reqdata, m.address)
	return err
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
