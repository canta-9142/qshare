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
	"github.com/canta-9142/qshare/internal/session"
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
	sess *session.Session,
	srv sessionServer,
) (sessionServer, string, error) {
	initialPort, err := a.selectServerPort()
	if err != nil {
		return nil, "", err
	}
	if initialPort < minimumServerPort || initialPort >= minimumServerPort+serverPortCount {
		return nil, "", fmt.Errorf("selected server port %d is outside the configured range", initialPort)
	}

	var listenAddr net.Addr
	// A random starting point keeps normal selection unpredictable. Advancing
	// within the range guarantees that collision retries do not repeat a port.
	for attempt := 0; attempt < serverPortAttempts; attempt++ {
		port := minimumServerPort + (int(initialPort)-minimumServerPort+attempt)%serverPortCount
		bindAddr := net.JoinHostPort(endpoint.Address.String(), strconv.FormatUint(uint64(port), 10))
		listenAddr, err = srv.Start(bindAddr)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, "", err
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf(
			"failed to find an available port in %d-%d after %d attempts: %w",
			minimumServerPort,
			minimumServerPort+serverPortCount-1,
			serverPortAttempts,
			err,
		)
	}

	_, portText, err := net.SplitHostPort(listenAddr.String())
	if err != nil {
		return nil, "", errors.Join(
			fmt.Errorf("failed to parse listen address: %w", err),
			srv.Close(),
		)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		if err == nil {
			err = errors.New("port must not be zero")
		}
		return nil, "", errors.Join(
			fmt.Errorf("failed to parse listen port %q: %w", portText, err),
			srv.Close(),
		)
	}

	lease, err := a.openFirewall(ctx, firewall.Rule{
		Interface:   endpoint.Interface,
		Source:      endpoint.Prefix,
		Destination: endpoint.Address,
		Port:        uint16(port),
		Timeout:     time.Until(sess.ExpiresAt()) + expirationDrainTimeout + firewallTimeoutSlack,
	})
	if err != nil {
		return nil, "", errors.Join(
			fmt.Errorf("failed to configure firewall: %w", err),
			srv.Close(),
		)
	}

	return &firewalledSessionServer{
		sessionServer: srv,
		lease:         lease,
	}, portText, nil
}

// firewalledSessionServer couples HTTP shutdown with firewall cleanup.
type firewalledSessionServer struct {
	sessionServer
	lease firewallLease
}

// Shutdown gracefully stops HTTP traffic and removes the firewall rule.
func (s *firewalledSessionServer) Shutdown(ctx context.Context) error {
	return errors.Join(s.sessionServer.Shutdown(ctx), s.closeFirewall())
}

// Close immediately stops HTTP traffic and removes the firewall rule.
func (s *firewalledSessionServer) Close() error {
	return errors.Join(s.sessionServer.Close(), s.closeFirewall())
}

// closeFirewall bounds cleanup independently from the session context.
func (s *firewalledSessionServer) closeFirewall() error {
	ctx, cancel := context.WithTimeout(context.Background(), firewallCleanupTimeout)
	defer cancel()
	if err := s.lease.Close(ctx); err != nil {
		return fmt.Errorf("remove temporary firewall rule: %w", err)
	}
	return nil
}
