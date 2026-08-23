//go:build linux

package firewall

import (
	"context"
	"os"
	"strings"
)

// nixOSBackendKind identifies a standard NixOS firewall implementation.
type nixOSBackendKind string

// NixOS backend and ruleset names mirror the standard NixOS firewall layout.
const (
	nixOSIPTablesBackend nixOSBackendKind = "nixos-iptables"
	nixOSNFTablesBackend nixOSBackendKind = "nixos-nftables"

	// These names are part of the standard NixOS firewall layouts used by
	// nixos-firewall-tool and the NixOS firewall modules.
	nixOSFirewallTable       = "nixos-fw"
	nixOSIPTablesAcceptChain = "nixos-fw-accept"
	nixOSNFTablesFamily      = "inet"
	nixOSNFTablesInputChain  = "input-allow"
)

// privilegedHelperLauncher starts a helper for a selected NixOS backend.
type privilegedHelperLauncher interface {
	start(context.Context, nixOSBackendKind, Rule) (Lease, error)
}

// nixOSBackend detects the active standard NixOS firewall service.
type nixOSBackend struct {
	readFile   func(string) ([]byte, error)
	pathExists func(string) bool
	launcher   privilegedHelperLauncher
}

// newNixOSBackend constructs a detector with production filesystem access.
func newNixOSBackend(runner commandRunner) *nixOSBackend {
	return &nixOSBackend{
		readFile: os.ReadFile,
		pathExists: func(path string) bool {
			_, err := os.Stat(path)
			return err == nil
		},
		launcher: newProcessHelperLauncher(runner),
	}
}

// tryOpen delegates a rule to the standard NixOS firewall when detected.
func (b *nixOSBackend) tryOpen(ctx context.Context, rule Rule) (Lease, bool, error) {
	osRelease, err := b.readFile("/etc/os-release")
	if err != nil || !isNixOSRelease(string(osRelease)) {
		return nil, false, nil
	}

	// Match nixos-firewall-tool's backend detection order. A NixOS system
	// configured to use firewalld has already been handled by the preceding
	// backend.
	var kind nixOSBackendKind
	switch {
	case b.pathExists("/etc/systemd/system/firewall.service"):
		kind = nixOSIPTablesBackend
	case b.pathExists("/etc/systemd/system/nftables.service"):
		kind = nixOSNFTablesBackend
	default:
		return nil, false, nil
	}

	lease, err := b.launcher.start(ctx, kind, rule)
	return lease, true, err
}

// isNixOSRelease recognizes NixOS from os-release contents.
func isNixOSRelease(contents string) bool {
	for line := range strings.SplitSeq(contents, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || key != "ID" {
			continue
		}
		return strings.Trim(value, "\"'") == "nixos"
	}
	return false
}
