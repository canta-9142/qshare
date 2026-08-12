//go:build linux

package network

import (
	"bufio"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"
)

var excludedInterfaceKeywords = []string{
	"docker", "veth", "br-", "kube", "cni",
	"tun", "tap", "wg", "wireguard", "tailscale",
	"vboxnet", "vmnet", "virbr", "vswif",
	"dummy", "team",
}

func advertiseAddress() (netip.Addr, error) {
	ifaces, err := getValidInterfaces()
	if err != nil {
		return netip.Addr{}, err
	}

	ifaces = prioritizeDefaultRouteInterfaces(ifaces)

	for _, iface := range ifaces {
		addr, ok := firstIPv4Address(iface)
		if ok {
			return addr, nil
		}
	}

	return netip.Addr{}, ErrNoLANAddress
}

func getValidInterfaces() ([]net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	validIfaces := make([]net.Interface, 0, len(ifaces))

	for _, iface := range ifaces {
		if !isValidInterface(iface) {
			continue
		}

		validIfaces = append(validIfaces, iface)
	}

	return validIfaces, nil
}

func isValidInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}

	if iface.Flags&net.FlagUp == 0 {
		return false
	}

	if iface.Flags&net.FlagBroadcast == 0 {
		return false
	}

	for _, keyword := range excludedInterfaceKeywords {
		if strings.HasPrefix(iface.Name, keyword) {
			return false
		}
	}

	if len(iface.HardwareAddr) == 0 {
		return false
	}

	mac := iface.HardwareAddr.String()
	if mac == "00:00:00:00:00:00" ||
		mac == "ff:ff:ff:ff:ff:ff" {
		return false
	}

	return true
}

func firstIPv4Address(iface net.Interface) (netip.Addr, bool) {
	addrs, err := iface.Addrs()
	if err != nil {
		return netip.Addr{}, false
	}

	for _, addr := range addrs {
		ip, ok := ipv4FromNetAddr(addr)
		if ok {
			return ip, true
		}
	}

	return netip.Addr{}, false
}

func ipv4FromNetAddr(addr net.Addr) (netip.Addr, bool) {
	var ip net.IP

	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return netip.Addr{}, false
	}

	netipAddr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}

	// net.IP internally represents IPv4 as IPv4-mapped IPv6 in some cases.
	netipAddr = netipAddr.Unmap()

	if !netipAddr.Is4() {
		return netip.Addr{}, false
	}

	if netipAddr.IsLoopback() ||
		netipAddr.IsUnspecified() ||
		netipAddr.IsMulticast() {
		return netip.Addr{}, false
	}

	return netipAddr, true
}

func prioritizeDefaultRouteInterfaces(ifaces []net.Interface) []net.Interface {
	defaultMetrics, err := readDefaultRouteMetrics()
	if err != nil {
		// Route information is only used for prioritization.
		// If it cannot be read, preserve the original order.
		return append([]net.Interface(nil), ifaces...)
	}

	sorted := append([]net.Interface(nil), ifaces...)

	sort.SliceStable(sorted, func(i, j int) bool {
		leftMetric, leftHasDefault := defaultMetrics[sorted[i].Name]
		rightMetric, rightHasDefault := defaultMetrics[sorted[j].Name]

		// Always prefer an interface with a default IPv4 route.
		if leftHasDefault != rightHasDefault {
			return leftHasDefault
		}

		// Preserve the original order among fallback interfaces.
		if !leftHasDefault {
			return false
		}

		// When multiple default routes exist, prefer the lower metric.
		return leftMetric < rightMetric
	})

	return sorted
}

func readDefaultRouteMetrics() (map[string]int, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/net/route: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Skip header.
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read /proc/net/route: %w", err)
		}
		return map[string]int{}, nil
	}

	metrics := make(map[string]int)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}

		destination := fields[1]
		flagsText := fields[3]
		metricText := fields[6]
		mask := fields[7]

		// Only consider default IPv4 routes.
		if destination != "00000000" || mask != "00000000" {
			continue
		}

		flags, err := strconv.ParseUint(flagsText, 16, 32)
		if err != nil {
			continue
		}

		// RTF_UP
		if flags&0x1 == 0 {
			continue
		}

		metric, err := strconv.Atoi(metricText)
		if err != nil {
			continue
		}

		ifaceName := fields[0]
		currentMetric, exists := metrics[ifaceName]
		if !exists || metric < currentMetric {
			metrics[ifaceName] = metric
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan /proc/net/route: %w", err)
	}

	return metrics, nil
}
