// Package firewall manages temporary firewall access for a qshare session.
//
// On Linux, it uses an active firewalld service first. On NixOS without
// firewalld, it falls back to the firewall implementation selected by NixOS:
// nftables or iptables. Other systems are left unchanged.
//
// Each rule is limited to the selected network interface, source subnet,
// destination address, TCP port, and session lifetime. NixOS table changes are
// made by a short-lived privileged helper. The helper removes its rule when the
// parent exits, and the rule also stops matching after its deadline if cleanup
// cannot run.
//
// To avoid elevating a replaceable executable, an unprivileged qshare process
// may start the helper only from a root-owned, non-replaceable installation
// path.
package firewall
