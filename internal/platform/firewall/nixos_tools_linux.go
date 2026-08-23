//go:build linux

package firewall

import (
	"fmt"
	"os"
)

// systemTool resolves a firewall tool from the active NixOS system or PATH.
func systemTool(runner commandRunner, name string) (string, error) {
	// Prefer the tool selected by the active NixOS system generation. PATH is a
	// fallback for configurations that do not expose it through the system path.
	path := "/run/current-system/sw/bin/" + name
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	path, err := runner.lookPath(name)
	if err != nil {
		return "", fmt.Errorf("find %s: %w", name, err)
	}
	return path, nil
}
