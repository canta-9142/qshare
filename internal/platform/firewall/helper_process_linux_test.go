//go:build linux

package firewall

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestValidatePrivilegedExecutableAllowsRootOwnedStickyDirectory(t *testing.T) {
	const executable = "/nix/store/example-qshare/bin/qshare"
	stat := fakePrivilegedPathStat(map[string]os.FileInfo{
		executable:   fakePrivilegedPathInfo{name: "qshare", mode: 0o555, uid: 0},
		"/nix/store": fakePrivilegedPathInfo{name: "store", mode: os.ModeDir | os.ModeSticky | 0o775, uid: 0},
	})

	if err := validatePrivilegedExecutable(executable, stat); err != nil {
		t.Fatalf("validatePrivilegedExecutable() error = %v", err)
	}
}

func TestValidatePrivilegedExecutableRejectsReplaceablePath(t *testing.T) {
	const executable = "/nix/store/example-qshare/bin/qshare"
	tests := []struct {
		name     string
		override map[string]os.FileInfo
	}{
		{
			name: "writable executable",
			override: map[string]os.FileInfo{
				executable: fakePrivilegedPathInfo{name: "qshare", mode: 0o775, uid: 0},
			},
		},
		{
			name: "writable directory without sticky bit",
			override: map[string]os.FileInfo{
				"/nix/store": fakePrivilegedPathInfo{name: "store", mode: os.ModeDir | 0o775, uid: 0},
			},
		},
		{
			name: "writable regular file with sticky bit",
			override: map[string]os.FileInfo{
				executable: fakePrivilegedPathInfo{name: "qshare", mode: os.ModeSticky | 0o775, uid: 0},
			},
		},
		{
			name: "non-root owner",
			override: map[string]os.FileInfo{
				"/nix/store": fakePrivilegedPathInfo{name: "store", mode: os.ModeDir | os.ModeSticky | 0o777, uid: 1000},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			override := map[string]os.FileInfo{
				executable: fakePrivilegedPathInfo{name: "qshare", mode: 0o555, uid: 0},
			}
			for path, info := range tt.override {
				override[path] = info
			}
			if err := validatePrivilegedExecutable(executable, fakePrivilegedPathStat(override)); err == nil {
				t.Fatal("validatePrivilegedExecutable() error = nil")
			}
		})
	}
}

type fakePrivilegedPathInfo struct {
	name string
	mode os.FileMode
	uid  uint32
}

func (i fakePrivilegedPathInfo) Name() string       { return i.name }
func (i fakePrivilegedPathInfo) Size() int64        { return 0 }
func (i fakePrivilegedPathInfo) Mode() os.FileMode  { return i.mode }
func (i fakePrivilegedPathInfo) ModTime() time.Time { return time.Time{} }
func (i fakePrivilegedPathInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakePrivilegedPathInfo) Sys() any           { return &syscall.Stat_t{Uid: i.uid} }

func fakePrivilegedPathStat(overrides map[string]os.FileInfo) func(string) (os.FileInfo, error) {
	return func(path string) (os.FileInfo, error) {
		if info, ok := overrides[path]; ok {
			return info, nil
		}
		return fakePrivilegedPathInfo{name: path, mode: os.ModeDir | 0o755, uid: 0}, nil
	}
}
