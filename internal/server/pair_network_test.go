package server

import (
	"net"
	"testing"
)

func TestInterfaceInfoSubnet(t *testing.T) {
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ifc := net.Interface{HardwareAddr: mac, Flags: net.FlagUp | net.FlagBroadcast}
	for _, tc := range []struct {
		cidr string
		mask string
	}{
		{"192.168.1.129/24", "255.255.255.0"},
		{"192.168.2.129/23", "255.255.254.0"},
		{"10.20.30.40/16", "255.255.0.0"},
		{"192.168.1.129/25", "255.255.255.128"},
		{"192.168.1.129/30", "255.255.255.252"},
	} {
		t.Run(tc.cidr, func(t *testing.T) {
			ip, network, err := net.ParseCIDR(tc.cidr)
			if err != nil {
				t.Fatal(err)
			}
			network.IP = ip
			info := interfaceInfo(ifc, []net.Addr{network}, ip)
			if info["mac"] != mac.String() || info["address"] != ip.String() || info["subnetMask"] != tc.mask {
				t.Fatalf("unexpected interface metadata: %v", info)
			}
		})
	}
}

func TestInterfaceInfoDoesNotGuessAnotherSubnet(t *testing.T) {
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ifc := net.Interface{HardwareAddr: mac, Flags: net.FlagUp | net.FlagBroadcast}
	firstIP, first, _ := net.ParseCIDR("192.168.1.129/24")
	secondIP, second, _ := net.ParseCIDR("10.20.30.40/16")
	first.IP, second.IP = firstIP, secondIP
	addrs := []net.Addr{first, second}
	info := interfaceInfo(ifc, addrs, secondIP)
	if info["address"] != "10.20.30.40" || info["subnetMask"] != "255.255.0.0" {
		t.Fatalf("wrong address selected: %v", info)
	}
	if info := interfaceInfo(ifc, addrs, net.ParseIP("192.168.10.13")); info != nil {
		t.Fatalf("invented a LAN for an unmatched address: %v", info)
	}
}

func TestInterfaceInfoOmitsNonBroadcastNetworks(t *testing.T) {
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	for _, cidr := range []string{"192.168.10.13/32", "192.168.1.128/31", "192.168.1.129/0", "fd00::1/64"} {
		ip, network, _ := net.ParseCIDR(cidr)
		network.IP = ip
		ifc := net.Interface{HardwareAddr: mac, Flags: net.FlagUp | net.FlagBroadcast}
		info := interfaceInfo(ifc, []net.Addr{network}, ip)
		if info["subnetMask"] != "" || info["address"] != "" {
			t.Fatalf("%s reported a broadcast subnet: %v", cidr, info)
		}
	}
	ip, network, _ := net.ParseCIDR("192.168.10.13/24")
	network.IP = ip
	for _, ifc := range []net.Interface{
		{Flags: net.FlagUp | net.FlagPointToPoint},
		{HardwareAddr: mac, Flags: net.FlagUp | net.FlagPointToPoint},
		{HardwareAddr: mac, Flags: net.FlagUp | net.FlagBroadcast | net.FlagLoopback},
	} {
		if info := interfaceInfo(ifc, []net.Addr{network}, ip); info["subnetMask"] != "" {
			t.Fatalf("non-broadcast interface reported a subnet: %v", info)
		}
	}
}
