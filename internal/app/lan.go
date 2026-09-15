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

// listenLAN reserves a port before firewall setup or HTTP serving begins.
func (a *Application) listenLAN(endpoint network.Endpoint) (net.Listener, uint16, error) {
	initialPort, err := a.selectServerPort()
	if err != nil {
		return nil, 0, err
	}
	if initialPort < minimumServerPort || initialPort >= minimumServerPort+serverPortCount {
		return nil, 0, fmt.Errorf("selected server port %d is outside the configured range", initialPort)
	}

	// A random starting point keeps normal selection unpredictable. Advancing
	// within the range guarantees that collision retries do not repeat a port.
	for attempt := 0; attempt < serverPortAttempts; attempt++ {
		port := minimumServerPort + (int(initialPort)-minimumServerPort+attempt)%serverPortCount
		bindAddr := net.JoinHostPort(endpoint.Address.String(), strconv.FormatUint(uint64(port), 10))
		var listener net.Listener
		listener, err = a.listen("tcp", bindAddr)
		if err == nil {
			return listener, uint16(port), nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, fmt.Errorf("listen: %w", err)
		}
	}
	return nil, 0, fmt.Errorf(
		"failed to find an available port in %d-%d after %d attempts: %w",
		minimumServerPort,
		minimumServerPort+serverPortCount-1,
		serverPortAttempts,
		err,
	)
}

func (a *Application) startLANServer(ctx context.Context, endpoint network.Endpoint, run *sessionRun) (uint16, error) {
	listener, port, err := a.listenLAN(endpoint)
	if err != nil {
		return 0, err
	}
	run.listener = listener
	run.lease, err = a.openFirewall(ctx, firewall.Rule{
		Interface:   endpoint.Interface,
		Source:      endpoint.Prefix,
		Destination: endpoint.Address,
		Port:        port,
		Timeout:     time.Until(run.session.ExpiresAt()) + expirationDrainTimeout + firewallTimeoutSlack,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to configure firewall: %w", err)
	}

	run.serve()
	return port, nil
}
