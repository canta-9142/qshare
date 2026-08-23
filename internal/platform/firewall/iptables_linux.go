//go:build linux

package firewall

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// iptablesLease owns one exact rule in the standard NixOS input chain.
type iptablesLease struct {
	runner     commandRunner
	executable string
	ruleArgs   []string
}

// openNixOSIPTables installs a scoped, expiring rule in the NixOS firewall chain.
func openNixOSIPTables(
	ctx context.Context,
	runner commandRunner,
	request helperRequest,
) (Lease, error) {
	executable, err := systemTool(runner, "iptables")
	if err != nil {
		return nil, err
	}
	maximum, _ := time.Parse(time.RFC3339, "2038-01-19T04:17:07Z")
	if request.expires.After(maximum) {
		return nil, errors.New("iptables temporary firewall expiration exceeds 2038 limit")
	}
	// xt_time makes the rule fail closed at the deadline even if explicit
	// cleanup cannot run.
	ruleArgs := []string{
		"-i", request.rule.Interface,
		"-s", request.rule.Source.Masked().String(),
		"-d", request.rule.Destination.String(),
		"-p", "tcp",
		"--dport", strconv.FormatUint(uint64(request.rule.Port), 10),
		"-m", "time",
		"--datestop", request.expires.UTC().Format("2006-01-02T15:04:05"),
		"-m", "comment",
		"--comment", "qshare:" + request.leaseID,
		"-j", nixOSIPTablesAcceptChain,
	}
	addArgs := append([]string{"-w", "-I", nixOSFirewallTable, "1"}, ruleArgs...)
	result := runner.run(ctx, executable, addArgs...)
	if result.err != nil {
		return nil, commandError("add qshare iptables rule", result)
	}
	return &iptablesLease{runner: runner, executable: executable, ruleArgs: ruleArgs}, nil
}

// Close removes the exact iptables rule owned by this lease.
func (l *iptablesLease) Close(ctx context.Context) error {
	removeArgs := append([]string{"-w", "-D", nixOSFirewallTable}, l.ruleArgs...)
	result := l.runner.run(ctx, l.executable, removeArgs...)
	if result.err == nil {
		return nil
	}
	checkArgs := append([]string{"-w", "-C", nixOSFirewallTable}, l.ruleArgs...)
	check := l.runner.run(ctx, l.executable, checkArgs...)
	if check.exitCode == 1 {
		return nil
	}
	return commandError("remove qshare iptables rule", result)
}
