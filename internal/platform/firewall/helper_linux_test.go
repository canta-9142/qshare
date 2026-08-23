//go:build linux

package firewall

import (
	"testing"
	"time"
)

func TestParseHelperRequest(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	expires := now.Add(10 * time.Minute)
	request, err := parseHelperRequest([]string{
		string(nixOSNFTablesBackend),
		"wlan0",
		"192.0.2.17/24",
		"192.0.2.23",
		"55544",
		"1787487000",
		"0123456789abcdef",
	}, now)
	if err != nil {
		t.Fatalf("parseHelperRequest() error = %v", err)
	}
	if request.backend != nixOSNFTablesBackend || request.rule.Interface != "wlan0" {
		t.Fatalf("request = %+v", request)
	}
	if request.rule.Source.String() != "192.0.2.0/24" {
		t.Errorf("source = %s, want 192.0.2.0/24", request.rule.Source)
	}
	if !request.expires.Equal(expires) || request.rule.Timeout != 10*time.Minute {
		t.Errorf("expires = %v, timeout = %v; want %v, 10m", request.expires, request.rule.Timeout, expires)
	}
}

func TestParseHelperRequestRejectsInvalidInputs(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	valid := []string{
		string(nixOSIPTablesBackend), "wlan0", "192.0.2.0/24", "192.0.2.23",
		"55544", "1787487000", "0123456789abcdef",
	}
	tests := []struct {
		name   string
		mutate func([]string) []string
	}{
		{name: "wrong count", mutate: func(args []string) []string { return args[:6] }},
		{name: "backend", mutate: func(args []string) []string { args[0] = "unknown"; return args }},
		{name: "interface syntax", mutate: func(args []string) []string { args[1] = `wlan0 accept`; return args }},
		{name: "source", mutate: func(args []string) []string { args[2] = "invalid"; return args }},
		{name: "destination", mutate: func(args []string) []string { args[3] = "198.51.100.1"; return args }},
		{name: "port", mutate: func(args []string) []string { args[4] = "0"; return args }},
		{name: "expired", mutate: func(args []string) []string { args[5] = "1"; return args }},
		{name: "lease ID", mutate: func(args []string) []string { args[6] = "../../bad"; return args }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := tt.mutate(append([]string(nil), valid...))
			if _, err := parseHelperRequest(args, now); err == nil {
				t.Fatal("parseHelperRequest() error = nil")
			}
		})
	}
}
