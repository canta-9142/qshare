//go:build linux

package firewall

import (
	"context"
	"errors"
	"testing"
)

type fakeHelperLauncher struct {
	kind  nixOSBackendKind
	rule  Rule
	lease Lease
	err   error
	calls int
}

func (l *fakeHelperLauncher) start(_ context.Context, kind nixOSBackendKind, rule Rule) (Lease, error) {
	l.calls++
	l.kind = kind
	l.rule = rule
	return l.lease, l.err
}

func TestNixOSBackendUsesIPTablesServiceFirst(t *testing.T) {
	launcher := &fakeHelperLauncher{lease: noopLease{}}
	backend := &nixOSBackend{
		readFile: func(string) ([]byte, error) { return []byte("NAME=NixOS\nID=nixos\n"), nil },
		pathExists: func(path string) bool {
			return path == "/etc/systemd/system/firewall.service" ||
				path == "/etc/systemd/system/nftables.service"
		},
		launcher: launcher,
	}

	_, handled, err := backend.tryOpen(context.Background(), testRule())
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if !handled {
		t.Fatal("tryOpen() handled = false, want true")
	}
	if launcher.kind != nixOSIPTablesBackend {
		t.Fatalf("helper backend = %q, want %q", launcher.kind, nixOSIPTablesBackend)
	}
}

func TestNixOSBackendUsesNFTablesService(t *testing.T) {
	launcher := &fakeHelperLauncher{lease: noopLease{}}
	backend := &nixOSBackend{
		readFile: func(string) ([]byte, error) { return []byte("ID='nixos'\n"), nil },
		pathExists: func(path string) bool {
			return path == "/etc/systemd/system/nftables.service"
		},
		launcher: launcher,
	}

	_, handled, err := backend.tryOpen(context.Background(), testRule())
	if err != nil {
		t.Fatalf("tryOpen() error = %v", err)
	}
	if !handled || launcher.kind != nixOSNFTablesBackend {
		t.Fatalf("handled = %v, helper backend = %q", handled, launcher.kind)
	}
}

func TestNixOSBackendIgnoresOtherSystemsAndUnknownFirewall(t *testing.T) {
	tests := []struct {
		name      string
		osRelease []byte
		readErr   error
		exists    bool
	}{
		{name: "other distribution", osRelease: []byte("ID=fedora\n"), exists: true},
		{name: "unreadable release", readErr: errors.New("denied"), exists: true},
		{name: "no standard service", osRelease: []byte("ID=nixos\n")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launcher := &fakeHelperLauncher{}
			backend := &nixOSBackend{
				readFile:   func(string) ([]byte, error) { return tt.osRelease, tt.readErr },
				pathExists: func(string) bool { return tt.exists },
				launcher:   launcher,
			}
			_, handled, err := backend.tryOpen(context.Background(), testRule())
			if err != nil {
				t.Fatalf("tryOpen() error = %v", err)
			}
			if handled || launcher.calls != 0 {
				t.Fatalf("handled = %v, helper calls = %d; want false, 0", handled, launcher.calls)
			}
		})
	}
}
