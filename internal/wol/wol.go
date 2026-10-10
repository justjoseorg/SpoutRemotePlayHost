// Package wol sends Wake-on-LAN magic packets, so a host on the home LAN can wake
// another PC for a device that is away (for example over WireGuard, where
// broadcasts don't cross the tunnel).
package wol

import (
	"bytes"
	"errors"
	"net"
)

// ParseMAC accepts a 48-bit MAC in any of the usual notations.
func ParseMAC(s string) (net.HardwareAddr, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return nil, errors.New("invalid MAC address")
	}
	return hw, nil
}

// Packet is 6 bytes of 0xFF followed by the MAC 16 times.
func Packet(mac net.HardwareAddr) []byte {
	return append(bytes.Repeat([]byte{0xFF}, 6), bytes.Repeat(mac, 16)...)
}

// Broadcasts lists 255.255.255.255 and the broadcast address of every IPv4 subnet
// on an up, broadcast-capable interface. 255.255.255.255 alone follows the default
// route, which a full-tunnel VPN takes over.
func Broadcasts() []net.IP {
	out := []net.IP{net.IPv4bcast}
	ifs, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagBroadcast == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if b := broadcastOf(a); b != nil {
				out = append(out, b)
			}
		}
	}
	return out
}

func broadcastOf(a net.Addr) net.IP {
	n, ok := a.(*net.IPNet)
	if !ok {
		return nil
	}
	ip4 := n.IP.To4()
	if ip4 == nil || len(n.Mask) != net.IPv4len {
		return nil
	}
	ones, _ := n.Mask.Size()
	if ones >= 31 {
		return nil
	}
	b := make(net.IP, net.IPv4len)
	for i := range b {
		b[i] = ip4[i] | ^n.Mask[i]
	}
	return b
}

// Send sends the magic packet to every broadcast address on UDP ports 9 and 7.
// It fails only if nothing could be sent.
func Send(mac net.HardwareAddr) error {
	pkt := Packet(mac)
	var lastErr error
	sent := 0
	for _, ip := range Broadcasts() {
		for _, port := range []int{9, 7} {
			c, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: ip, Port: port})
			if err != nil {
				lastErr = err
				continue
			}
			if _, err := c.Write(pkt); err != nil {
				lastErr = err
			} else {
				sent++
			}
			c.Close()
		}
	}
	if sent == 0 {
		return lastErr
	}
	return nil
}
