//go:build linux

package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

// nftablesLease owns one qshare rule and its timed source set.
type nftablesLease struct {
	runner     commandRunner
	executable string
	handle     uint64
	setName    string
}

// openNixOSNFTables installs a scoped rule in the standard NixOS nftables table.
func openNixOSNFTables(
	ctx context.Context,
	runner commandRunner,
	request helperRequest,
) (Lease, error) {
	executable, err := systemTool(runner, "nft")
	if err != nil {
		return nil, err
	}
	setName := "qshare_" + request.leaseID
	// The source lives in a dedicated timed set. If the helper is killed before
	// cleanup, the remaining rule stops matching when the element expires.
	result := runner.run(
		ctx,
		executable,
		"add", "set", nixOSNFTablesFamily, nixOSFirewallTable, setName,
		"{", "type", "ipv4_addr", ";", "flags", "interval,timeout", ";", "}",
	)
	if result.err != nil {
		return nil, commandError("create qshare nftables set", result)
	}

	cleanupSet := func() {
		_ = runner.run(
			context.Background(), executable,
			"delete", "set", nixOSNFTablesFamily, nixOSFirewallTable, setName,
		)
	}
	seconds := timeoutSeconds(time.Until(request.expires))
	result = runner.run(
		ctx,
		executable,
		"add", "element", nixOSNFTablesFamily, nixOSFirewallTable, setName,
		"{", request.rule.Source.Masked().String(), "timeout", strconv.FormatInt(seconds, 10)+"s", "}",
	)
	if result.err != nil {
		cleanupSet()
		return nil, commandError("add qshare nftables source", result)
	}

	comment := "qshare:" + request.leaseID
	result = runner.run(
		ctx,
		executable,
		"--json", "--echo", "--handle",
		"insert", "rule", nixOSNFTablesFamily, nixOSFirewallTable, nixOSNFTablesInputChain,
		"iifname", request.rule.Interface,
		"ip", "saddr", "@"+setName,
		"ip", "daddr", request.rule.Destination.String(),
		"tcp", "dport", strconv.FormatUint(uint64(request.rule.Port), 10),
		"accept", "comment", strconv.Quote(comment),
	)
	if result.err != nil {
		cleanupSet()
		return nil, commandError("add qshare nftables rule", result)
	}
	handle, ok := nftRuleHandle(result.output, comment)
	if !ok {
		// Emptying the set makes the rule fail closed even if its handle cannot
		// be recovered for immediate cleanup.
		_ = runner.run(
			context.Background(), executable,
			"flush", "set", nixOSNFTablesFamily, nixOSFirewallTable, setName,
		)
		return nil, errors.New("nft did not report the qshare rule handle")
	}

	return &nftablesLease{
		runner:     runner,
		executable: executable,
		handle:     handle,
		setName:    setName,
	}, nil
}

// Close removes the nftables rule followed by its source set.
func (l *nftablesLease) Close(ctx context.Context) error {
	// Remove the referencing rule before its set to preserve nftables ordering.
	removeRule := l.runner.run(
		ctx,
		l.executable,
		"delete", "rule", nixOSNFTablesFamily, nixOSFirewallTable,
		nixOSNFTablesInputChain, "handle", strconv.FormatUint(l.handle, 10),
	)
	removeSet := l.runner.run(
		ctx,
		l.executable,
		"delete", "set", nixOSNFTablesFamily, nixOSFirewallTable, l.setName,
	)
	var ruleErr, setErr error
	if removeRule.err != nil {
		ruleErr = commandError("remove qshare nftables rule", removeRule)
	}
	if removeSet.err != nil {
		setErr = commandError("remove qshare nftables set", removeSet)
	}
	return errors.Join(ruleErr, setErr)
}

// nftRuleHandle extracts the owned rule handle from nft JSON output.
func nftRuleHandle(output, comment string) (uint64, bool) {
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return 0, false
	}
	return findNFTRuleHandle(value, comment)
}

// findNFTRuleHandle recursively finds a rule with the expected ownership comment.
func findNFTRuleHandle(value any, comment string) (uint64, bool) {
	switch current := value.(type) {
	case []any:
		for _, item := range current {
			if handle, ok := findNFTRuleHandle(item, comment); ok {
				return handle, true
			}
		}
	case map[string]any:
		if currentComment, _ := current["comment"].(string); currentComment == comment {
			if number, ok := current["handle"].(json.Number); ok {
				handle, err := strconv.ParseUint(number.String(), 10, 64)
				return handle, err == nil
			}
		}
		for _, item := range current {
			if handle, ok := findNFTRuleHandle(item, comment); ok {
				return handle, true
			}
		}
	}
	return 0, false
}
