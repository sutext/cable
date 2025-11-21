package muticast

import (
	"fmt"
	"net"
	"sync"

	"sutext.github.io/cable/packet"
)

type Muticast interface {
	Stop()
	Serve() error
	Request()
}

type muticast struct {
	id           string
	ip           string
	addr         *net.UDPAddr
	conn         *net.UDPConn
	requestTasks sync.Map
}

func New(id string) (*muticast, error) {
	ip, err := getLocalIP()
	if err != nil {
		return nil, err
	}
	m := &muticast{
		id:   id,
		ip:   ip,
		addr: &net.UDPAddr{IP: net.ParseIP("224.0.0.99"), Port: 9999},
	}
	return m, nil
}
func (m *muticast) Serve() error {
	go m.listen()
	conn, err := net.ListenMulticastUDP("udp4", nil, m.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	buffer := make([]byte, 65535)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			fmt.Println(err)
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			fmt.Println(err)
			return err
		}
		if p.Type() != packet.REQUEST {
			continue
		}
		req := p.(*packet.Request)
		if req.Method != "discovery" {
			continue
		}
		resp := packet.NewResponse(req.ID, []byte(m.id))
		resp.Content = []byte(m.ip)
		bytes, err := packet.Marshal(resp)
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println(addr, req.ID, string(req.Content))
		_, err = conn.WriteToUDP(bytes, addr)
		if err != nil {
			fmt.Println(err)
			return err
		}
	}
}
func (m *muticast) listen() error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	m.conn = conn
	defer conn.Close()
	buffer := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			return err
		}
		p, err := packet.Unmarshal(buffer[:n])
		if err != nil {
			return err
		}
		if p.Type() != packet.RESPONSE {
			continue
		}
		resp := p.(*packet.Response)
		if ch, ok := m.requestTasks.Load(resp.ID); ok {
			ch.(chan *packet.Response) <- resp
		}
	}
}
func (m *muticast) Request(req *packet.Request) (resp []*packet.Response, err error) {
	reqdata, err := packet.Marshal(req)
	if err != nil {
		return nil, err
	}

	respChan := make(chan *packet.Response)
	defer close(respChan)
	m.requestTasks.Store(req.ID, respChan)
	defer m.requestTasks.Delete(req.ID)
	_, err = m.conn.WriteToUDP(reqdata, m.addr)
	if err != nil {
		return nil, err
	}
	for res := range respChan {
		resp = append(resp, res)
	}
	return resp, nil
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

type udpReader struct {
	conn *net.UDPConn
}

func (r *udpReader) Read(p []byte) (n int, err error) {
	return r.conn.Read(p)
}
