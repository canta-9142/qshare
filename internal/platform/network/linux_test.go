//go:build linux

package network

import (
	"net"
	"net/netip"
	"strings"
	"testing"
)

func TestIsValidInterface(t *testing.T) {
	valid := net.Interface{Name: "eth0", HardwareAddr: net.HardwareAddr{2, 0, 0, 0, 0, 1}, Flags: net.FlagUp | net.FlagBroadcast}
	tests := []struct {
		name   string
		mutate func(*net.Interface)
		want   bool
	}{
		{name: "valid", want: true},
		{name: "loopback", mutate: func(i *net.Interface) { i.Flags |= net.FlagLoopback }},
		{name: "down", mutate: func(i *net.Interface) { i.Flags &^= net.FlagUp }},
		{name: "no broadcast", mutate: func(i *net.Interface) { i.Flags &^= net.FlagBroadcast }},
		{name: "excluded", mutate: func(i *net.Interface) { i.Name = "docker0" }},
		{name: "no MAC", mutate: func(i *net.Interface) { i.HardwareAddr = nil }},
		{name: "zero MAC", mutate: func(i *net.Interface) { i.HardwareAddr = net.HardwareAddr{0, 0, 0, 0, 0, 0} }},
		{name: "broadcast MAC", mutate: func(i *net.Interface) { i.HardwareAddr = net.HardwareAddr{255, 255, 255, 255, 255, 255} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iface := valid
			if tt.mutate != nil {
				tt.mutate(&iface)
			}
			if got := isValidInterface(iface); got != tt.want {
				t.Fatalf("isValidInterface() = %v, want %v", got, tt.want)
			}
		})
	}
}

type unusualAddr string

func (a unusualAddr) Network() string { return "test" }
func (a unusualAddr) String() string  { return string(a) }

func TestIPv4FromNetAddr(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want string
		ok   bool
	}{
		{name: "IPNet", addr: &net.IPNet{IP: net.ParseIP("192.0.2.1"), Mask: net.CIDRMask(24, 32)}, want: "192.0.2.1", ok: true},
		{name: "IPAddr", addr: &net.IPAddr{IP: net.ParseIP("198.51.100.2")}, want: "198.51.100.2", ok: true},
		{name: "IPv6", addr: &net.IPAddr{IP: net.ParseIP("2001:db8::1")}},
		{name: "loopback", addr: &net.IPAddr{IP: net.ParseIP("127.0.0.1")}},
		{name: "unspecified", addr: &net.IPAddr{IP: net.ParseIP("0.0.0.0")}},
		{name: "multicast", addr: &net.IPAddr{IP: net.ParseIP("224.0.0.1")}},
		{name: "unknown", addr: unusualAddr("192.0.2.1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ipv4FromNetAddr(tt.addr)
			if ok != tt.ok || (ok && got.String() != tt.want) {
				t.Fatalf("ipv4FromNetAddr() = %v, %v; want %s, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestIPv4PrefixFromNetAddr(t *testing.T) {
	tests := []struct {
		name string
		addr net.Addr
		want string
		ok   bool
	}{
		{
			name: "IPNet",
			addr: &net.IPNet{
				IP:   net.ParseIP("192.0.2.23"),
				Mask: net.CIDRMask(24, 32),
			},
			want: "192.0.2.0/24",
			ok:   true,
		},
		{
			name: "IPAddr",
			addr: &net.IPAddr{IP: net.ParseIP("198.51.100.2")},
			want: "198.51.100.2/32",
			ok:   true,
		},
		{
			name: "invalid mask",
			addr: &net.IPNet{
				IP:   net.ParseIP("192.0.2.23"),
				Mask: net.IPMask{255, 0, 255, 0},
			},
		},
		{
			name: "IPv6",
			addr: &net.IPNet{
				IP:   net.ParseIP("2001:db8::1"),
				Mask: net.CIDRMask(64, 128),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ipv4PrefixFromNetAddr(tt.addr)
			if ok != tt.ok {
				t.Fatalf("ipv4PrefixFromNetAddr() ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != netip.MustParsePrefix(tt.want) {
				t.Fatalf("ipv4PrefixFromNetAddr() = %v, want %s", got, tt.want)
			}
		})
	}
}

func TestPrioritizeDefaultRouteInterfaces(t *testing.T) {
	in := []net.Interface{{Name: "fallback"}, {Name: "slow"}, {Name: "fast"}}
	original := append([]net.Interface(nil), in...)
	got := prioritizeInterfaces(in, map[string]int{"slow": 200, "fast": 10})
	want := []string{"fast", "slow", "fallback"}
	for i := range want {
		if got[i].Name != want[i] {
			t.Fatalf("prioritized[%d] = %q, want %q", i, got[i].Name, want[i])
		}
	}
	if in[0].Name != original[0].Name || in[1].Name != original[1].Name || in[2].Name != original[2].Name {
		t.Fatal("input slice was mutated")
	}
}

func TestParseDefaultRouteMetrics(t *testing.T) {
	routes := "Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\n" +
		"eth0 00000000 00000000 0001 0 0 200 00000000 0 0 0\n" +
		"eth0 00000000 00000000 0001 0 0 50 00000000 0 0 0\n" +
		"wlan0 00000000 00000000 0000 0 0 10 00000000 0 0 0\n" +
		"eth1 00000001 00000000 0001 0 0 1 FFFFFFFF 0 0 0\n" +
		"broken row\n"
	got, err := parseDefaultRouteMetrics(strings.NewReader(routes))
	if err != nil {
		t.Fatalf("parseDefaultRouteMetrics() error = %v", err)
	}
	want := map[string]int{"eth0": 50}
	if len(got) != len(want) || got["eth0"] != 50 {
		t.Fatalf("metrics = %v, want %v", got, want)
	}

	empty, err := parseDefaultRouteMetrics(strings.NewReader(""))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty metrics = %v, err=%v", empty, err)
	}
}
