package discovery

import (
	"fmt"
	"net"

	"sutext.github.io/cable/packet"
)

type Discovery struct {
	id string
	ip string
}

func New(id string) (*Discovery, error) {
	ip, err := getLocalIP()
	if err != nil {
		return nil, err
	}
	return &Discovery{id: id, ip: ip}, nil
}
func (d *Discovery) Start() error {
	conn, err := net.ListenMulticastUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP("224.0.0.24"), Port: 9999})
	if err != nil {
		return err
	}
	defer conn.Close()
	for {
		buf := make([]byte, 1024)
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		conn.WriteToUDP([]byte(d.id), addr)
		fmt.Println(addr.String(), string(buf[:n]))
		fmt.Println(string(buf[:n]))
	}
}
func (d *Discovery) Query() (id string, err error) {
	conn, err := net.Dial("udp4", "224.0.0.24:9999")
	if err != nil {
		return id, err
	}
	defer conn.Close()
	request := packet.NewRequest("discovery")
	err = packet.WriteTo(conn, request)
	if err != nil {
		return id, err
	}
	p, err := packet.ReadFrom(conn)
	if err != nil {
		return id, err
	}
	if p.Type() != packet.RESPONSE {
		return id, fmt.Errorf("unexpected packet type: %d", p.Type())
	}
	return string(p.(*packet.Response).Content), nil
}
func Lookup(name string) (net.IP, error) {
	ips, err := net.LookupIP(name)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if ip.To4() != nil {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("no IPv4 address found for %s", name)
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
