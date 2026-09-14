package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"syscall"
	"time"

	"github.com/canta-9142/qshare/internal/platform/firewall"
	"github.com/canta-9142/qshare/internal/platform/network"
)

// randomServerPort selects a uniformly distributed port from the configured range.
func randomServerPort() (uint16, error) {
	offset, err := rand.Int(rand.Reader, big.NewInt(serverPortCount))
	if err != nil {
		return 0, fmt.Errorf("select random server port: %w", err)
	}
	return uint16(minimumServerPort + offset.Int64()), nil
}

// startLANServer binds an available random port and opens its temporary firewall rule.
func (a *Application) startLANServer(
	ctx context.Context,
	endpoint network.Endpoint,
	run *sessionRun,
) (string, error) {
	initialPort, err := a.selectServerPort()
	if err != nil {
		return "", err
	}
	if initialPort < minimumServerPort || initialPort >= minimumServerPort+serverPortCount {
		return "", fmt.Errorf("selected server port %d is outside the configured range", initialPort)
	}

	var listenAddr net.Addr
	// A random starting point keeps normal selection unpredictable. Advancing
	// within the range guarantees that collision retries do not repeat a port.
	for attempt := 0; attempt < serverPortAttempts; attempt++ {
		port := minimumServerPort + (int(initialPort)-minimumServerPort+attempt)%serverPortCount
		bindAddr := net.JoinHostPort(endpoint.Address.String(), strconv.FormatUint(uint64(port), 10))
		listenAddr, err = run.server.Start(bindAddr)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return "", err
		}
	}
	if err != nil {
		return "", fmt.Errorf(
			"failed to find an available port in %d-%d after %d attempts: %w",
			minimumServerPort,
			minimumServerPort+serverPortCount-1,
			serverPortAttempts,
			err,
		)
	}

	_, portText, err := net.SplitHostPort(listenAddr.String())
	if err != nil {
		return "", fmt.Errorf("failed to parse listen address: %w", err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		if err == nil {
			err = errors.New("port must not be zero")
		}
		return "", fmt.Errorf("failed to parse listen port %q: %w", portText, err)
	}

	run.lease, err = a.openFirewall(ctx, firewall.Rule{
		Interface:   endpoint.Interface,
		Source:      endpoint.Prefix,
		Destination: endpoint.Address,
		Port:        uint16(port),
		Timeout:     time.Until(run.session.ExpiresAt()) + expirationDrainTimeout + firewallTimeoutSlack,
	})
	if err != nil {
		return "", fmt.Errorf("failed to configure firewall: %w", err)
	}

	return portText, nil
}
