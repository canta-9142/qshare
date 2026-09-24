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
func (a *Application) listenLAN(endpoint network.Endpoint, requestedPort uint16) (net.Listener, uint16, error) {
	initialPort := requestedPort
	attempts := 1
	var err error
	if requestedPort == 0 {
		initialPort, err = a.selectServerPort()
		if err != nil {
			return nil, 0, err
		}
		if initialPort < minimumServerPort || initialPort >= minimumServerPort+serverPortCount {
			return nil, 0, fmt.Errorf("selected server port %d is outside the configured range", initialPort)
		}
		attempts = serverPortAttempts
	}

	// A random starting point keeps normal selection unpredictable. Advancing
	// within the range guarantees that collision retries do not repeat a port.
	for attempt := 0; attempt < attempts; attempt++ {
		port := int(initialPort)
		if requestedPort == 0 {
			port = minimumServerPort + (port-minimumServerPort+attempt)%serverPortCount
		}
		bindAddr := net.JoinHostPort(endpoint.Address.String(), strconv.FormatUint(uint64(port), 10))
		var listener net.Listener
		listener, err = a.listen("tcp", bindAddr)
		if err == nil {
			return listener, uint16(port), nil
		}
		if requestedPort != 0 {
			return nil, 0, fmt.Errorf("listen on port %d: %w", requestedPort, err)
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

func (a *Application) startLANServer(ctx context.Context, endpoint network.Endpoint, run *sessionRun, requestedPort uint16) (uint16, error) {
	listener, port, err := a.listenLAN(endpoint, requestedPort)
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
