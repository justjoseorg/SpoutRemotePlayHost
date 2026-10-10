package wol

import (
	"bytes"
	"net"
	"testing"
)

func TestPacket(t *testing.T) {
	mac, err := ParseMAC("10:ff:e0:32:86:cb")
	if err != nil {
		t.Fatal(err)
	}
	p := Packet(mac)
	if len(p) != 102 || !bytes.Equal(p[:6], []byte{255, 255, 255, 255, 255, 255}) || !bytes.Equal(p[96:], mac) {
		t.Fatalf("bad packet % x", p)
	}
}

func TestParseMACRejects(t *testing.T) {
	for _, s := range []string{"", "zz:ff:e0:32:86:cb", "00:00:5e:00:53:01:02:03"} {
		if _, err := ParseMAC(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestBroadcastOf(t *testing.T) {
	_, n, _ := net.ParseCIDR("192.168.1.129/24")
	n.IP = net.ParseIP("192.168.1.129").To4()
	if b := broadcastOf(n); b.String() != "192.168.1.255" {
		t.Fatalf("got %v", b)
	}
	_, wg, _ := net.ParseCIDR("192.168.10.13/32")
	if b := broadcastOf(wg); b != nil {
		t.Fatalf("/32 got %v", b)
	}
}
