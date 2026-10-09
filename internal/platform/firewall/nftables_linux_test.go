//go:build linux

package firewall

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestNFTRuleHandle(t *testing.T) {
	output := `{"nftables":[{"add":{"rule":{"family":"inet","table":"nixos-fw","chain":"input-allow","comment":"qshare:0123456789abcdef","handle":42}}}]}`
	handle, ok := nftRuleHandle(output, "qshare:0123456789abcdef")
	if !ok || handle != 42 {
		t.Fatalf("nftRuleHandle() = %d, %v; want 42, true", handle, ok)
	}
	if _, ok := nftRuleHandle(output, "qshare:other"); ok {
		t.Fatal("nftRuleHandle() found unrelated rule")
	}
}

func TestNFTRuleHandleResponseFormat(t *testing.T) {
	tests := []struct {
		name   string
		output string
		handle uint64
		ok     bool
	}{
		{"multiple objects", `{"nftables":[{"metainfo":{"json_schema_version":1}},{"add":{"rule":{"comment":"other","handle":1}}},{"add":{"rule":{"comment":"owned","handle":42}}}]}`, 42, true},
		{"zero handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":0}}}]}`, 0, true},
		{"maximum handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":18446744073709551615}}}]}`, ^uint64(0), true},
		{"missing handle", `{"nftables":[{"add":{"rule":{"comment":"owned"}}}]}`, 0, false},
		{"null handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":null}}}]}`, 0, false},
		{"string handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":"42"}}}]}`, 0, false},
		{"negative handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":-1}}}]}`, 0, false},
		{"fractional handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":1.5}}}]}`, 0, false},
		{"overflow handle", `{"nftables":[{"add":{"rule":{"comment":"owned","handle":18446744073709551616}}}]}`, 0, false},
		{"unrelated object", `{"nftables":[{"add":{"set":{"comment":"owned","handle":42}}}]}`, 0, false},
		{"nested fields", `{"nftables":[{"add":{"rule":{"expr":[{"comment":"owned","handle":42}]}}}]}`, 0, false},
		{"unwrapped rule", `{"nftables":[{"rule":{"comment":"owned","handle":42}}]}`, 0, false},
		{"empty response", `{}`, 0, false},
		{"invalid JSON", `{"nftables":`, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handle, ok := nftRuleHandle(tt.output, "owned")
			if handle != tt.handle || ok != tt.ok {
				t.Fatalf("nftRuleHandle() = %d, %v; want %d, %v", handle, ok, tt.handle, tt.ok)
			}
		})
	}
}

func TestOpenNixOSNFTablesAddsAndRemovesOwnedRule(t *testing.T) {
	request := helperRequest{
		backend: nixOSNFTablesBackend,
		rule:    testRule(),
		expires: time.Now().Add(10 * time.Minute),
		leaseID: "0123456789abcdef",
	}
	runner := &fakeRunner{path: "/usr/bin/nft", results: []commandResult{
		{},
		{},
		{output: `{"nftables":[{"add":{"rule":{"comment":"qshare:0123456789abcdef","handle":42}}}]}`},
		{},
		{},
	}}
	lease, err := openNixOSNFTables(context.Background(), runner, request)
	if err != nil {
		t.Fatalf("openNixOSNFTables() error = %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("command calls = %d, want 5: %#v", len(runner.calls), runner.calls)
	}
	if !slices.Equal(runner.calls[0].args, []string{
		"add", "set", "inet", "nixos-fw", "qshare_0123456789abcdef",
		"{", "type", "ipv4_addr", ";", "flags", "interval,timeout", ";", "}",
	}) {
		t.Errorf("add set args = %v", runner.calls[0].args)
	}
	addRule := runner.calls[2].args
	for _, want := range []string{"input-allow", "wlan0", "@qshare_0123456789abcdef", "192.0.2.23", "55544", `"qshare:0123456789abcdef"`} {
		if !slices.Contains(addRule, want) {
			t.Errorf("add rule args = %v, missing %q", addRule, want)
		}
	}
	if !slices.Equal(runner.calls[3].args, []string{
		"delete", "rule", "inet", "nixos-fw", "input-allow", "handle", "42",
	}) {
		t.Errorf("delete rule args = %v", runner.calls[3].args)
	}
	if !slices.Equal(runner.calls[4].args, []string{
		"delete", "set", "inet", "nixos-fw", "qshare_0123456789abcdef",
	}) {
		t.Errorf("delete set args = %v", runner.calls[4].args)
	}
}

func TestOpenNixOSNFTablesFailsClosedWithoutHandle(t *testing.T) {
	runner := &fakeRunner{path: "/usr/bin/nft", results: []commandResult{{}, {}, {output: `{}`}, {}}}
	request := helperRequest{rule: testRule(), expires: time.Now().Add(time.Minute), leaseID: "0123456789abcdef"}
	_, err := openNixOSNFTables(context.Background(), runner, request)
	if err == nil || !strings.Contains(err.Error(), "handle") {
		t.Fatalf("openNixOSNFTables() error = %v", err)
	}
	if len(runner.calls) != 4 || !slices.Equal(runner.calls[3].args, []string{
		"flush", "set", "inet", "nixos-fw", "qshare_0123456789abcdef",
	}) {
		t.Fatalf("cleanup call = %#v", runner.calls)
	}
}

func TestNftablesPartialFailurePreservesCleanupErrors(t *testing.T) {
	setupFailure := errors.New("nft setup failed")
	cleanupFailure := errors.New("nft cleanup failed")
	for _, stage := range []string{"source", "rule", "handle", "cancel"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			results := []commandResult{{}, {err: setupFailure}, {err: cleanupFailure}}
			cleanupAction := "delete"
			if stage == "rule" || stage == "handle" {
				results = []commandResult{{}, {}, {err: setupFailure}, {err: cleanupFailure}}
				cleanupAction = "flush"
			}
			if stage == "handle" {
				results[2] = commandResult{output: `{}`}
			}
			runner := &fakeRunner{
				results: results,
				onRun: func(commandCtx context.Context, args []string) {
					if stage == "cancel" && slices.Equal(args[:2], []string{"add", "element"}) {
						cancel()
					}
					if args[0] == cleanupAction {
						assertFirewallCleanupContext(t, commandCtx)
					}
				},
			}
			request := helperRequest{rule: testRule(), expires: time.Now().Add(time.Minute), leaseID: "0123456789abcdef"}
			_, err := openNixOSNFTables(ctx, runner, request)
			if !errors.Is(err, cleanupFailure) {
				t.Fatalf("cleanup failure was lost: %v", err)
			}
			if stage == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancellation was lost: %v", err)
				}
			} else if stage != "handle" && !errors.Is(err, setupFailure) {
				t.Fatalf("setup failure was lost: %v", err)
			}
			if len(runner.calls) != len(results) || runner.calls[len(results)-1].args[0] != cleanupAction {
				t.Fatalf("unexpected cleanup commands: %v", runner.calls)
			}
		})
	}
}
