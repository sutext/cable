package discovery

import (
	"fmt"
	"net"

	"sutext.github.io/cable/packet"
)

type Discovery struct {
	id   string
	ip   string
	addr *net.UDPAddr
}

func New(id string) (*Discovery, error) {
	ip, err := getLocalIP()
	if err != nil {
		return nil, err
	}
	return &Discovery{id: id, ip: ip, addr: &net.UDPAddr{IP: net.ParseIP("224.0.0.2"), Port: 9999}}, nil
}
func (d *Discovery) Start() error {
	conn, err := net.ListenMulticastUDP("udp4", nil, d.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		buf := make([]byte, 2048)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println(err)
			return err
		}
		fmt.Println("received from ", addr, "len:", n)
		p, err := packet.Unmarshal(buf[:n])
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
		resp := packet.NewResponse(req.ID, []byte(d.id))
		resp.Content = []byte(d.ip)
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
func (d *Discovery) Query() (id string, err error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 0})
	if err != nil {
		return id, err
	}
	defer conn.Close()
	req := packet.NewRequest("discovery")
	reqdata, err := packet.Marshal(req)
	if err != nil {
		return id, err
	}
	_, err = conn.WriteToUDP(reqdata, d.addr)
	if err != nil {
		return id, err
	}
	data := make([]byte, 2048)
	n, _, err := conn.ReadFromUDP(data)
	if err != nil {
		return id, err
	}
	p, err := packet.Unmarshal(data[:n])
	if err != nil {
		return id, err
	}
	if p.Type() != packet.RESPONSE {
		return id, fmt.Errorf("unexpected packet type: %d", p.Type())
	}
	return string(p.(*packet.Response).Content), nil
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
